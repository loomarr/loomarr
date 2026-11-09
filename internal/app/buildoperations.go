package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	guideapi "github.com/loomarr/loomarr/internal/api/guide"
	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/api/people"
	setupapi "github.com/loomarr/loomarr/internal/api/setup"
	"github.com/loomarr/loomarr/internal/api/system"
	"github.com/loomarr/loomarr/internal/app/nownext"
	"github.com/loomarr/loomarr/internal/app/playoutresolve"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/library/tmdb"
	"github.com/loomarr/loomarr/internal/people/access"
	"github.com/loomarr/loomarr/internal/people/auth"
	"github.com/loomarr/loomarr/internal/people/notifications"
	"github.com/loomarr/loomarr/internal/people/secretprotection"
	"github.com/loomarr/loomarr/internal/platform/config"
	"github.com/loomarr/loomarr/internal/platform/events"
	"github.com/loomarr/loomarr/internal/platform/httpx"
	"github.com/loomarr/loomarr/internal/platform/jobs"
	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/platform/metrics"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/playout/tunarr"
)

func buildRestart(overrides Overrides, log *slog.Logger) (system.RestartService, *config.Config) {
	var service system.RestartService
	if overrides.Restart != nil {
		service = restartAdapter{fn: overrides.Restart}
	}
	bootConfig, err := config.Load()
	if err != nil {
		log.Warn("could not read boot config for restart-required detection", "err", err)
		bootConfig = nil
	}
	return service, bootConfig
}

func buildDatabase(st store.Store, set resolved, overrides Overrides, eventBus *events.Bus) system.DatabaseService {
	if st == nil {
		return nil
	}
	return newDatabaseService(st, func() string { return settings.BackupDir.Get(set) }, eventBus).
		WithMigrationRequest(overrides.DatabaseMigration).
		WithLastError(overrides.DatabaseMigrationError)
}

type residentLLMBuild struct {
	probe func(context.Context) (float64, string)
}

type operationsBuild struct {
	backups                  system.BackupsService
	restart                  system.RestartService
	bootConfig               *config.Config
	auth                     authBuild
	guide                    guideapi.GuideReader
	settings                 setupapi.SettingsService
	notificationDestinations setupapi.NotificationDestinationService
	webPushPublicKey         string
	productNotifications     *productNotificationCoordinator
	invitationDelivery       people.InvitationDeliveryService
	passwordRecovery         people.PasswordRecoveryService
	liveSettings             settings.Reader
	libraryConfigured        func() bool
	jobs                     system.JobService
	// scheduler is nil without a store; Current Health reads starved tasks from it.
	scheduler   *jobs.Scheduler
	database    system.DatabaseService
	residentLLM residentLLMBuild
}

func buildOperations(
	rootCtx context.Context,
	st store.Store,
	set resolved,
	desiredSet resolved,
	secrets *settings.Secrets,
	readGeneratedSecret func(context.Context, settings.GeneratedSecret) (string, error),
	refreshSecretRedactor func(),
	libraryClient *library.Client,
	tmdbClient *tmdb.Client,
	eventBus *events.Bus,
	emitter *eventEmitter,
	registry *jobs.Registry,
	owner *generationLifecycle,
	playoutResolver *playoutresolve.Resolver,
	appliedBackend func(context.Context) (string, error),
	overrides Overrides,
	log *slog.Logger,
	metricRecorder *metrics.Recorder,
	protection *secretprotection.Manager,
	secretRedactor *settings.Redactor,
) (operationsBuild, error) {
	restart, bootConfig := buildRestart(overrides, log)
	backups := buildBackups(st, set, registry, log)
	authResult := buildAuth(st, set, secrets, readGeneratedSecret, libraryClient, log)
	invitationService, _ := authResult.invitations.(*access.Service)
	recoveryService := authResult.passwordRecovery
	destinationRepository := notifications.NewProtectedRepository(st, protection).
		WithCredentialRedactor(secretRedactor)
	if destinationRepository != nil {
		if err := destinationRepository.SeedCredentialRedaction(rootCtx); err != nil {
			return operationsBuild{}, fmt.Errorf("register notification provider credentials for redaction: %w", err)
		}
	}
	if err := migrateLegacySMTPProvider(rootCtx, st, destinationRepository, set); err != nil {
		return operationsBuild{}, fmt.Errorf("migrate SMTP notification provider: %w", err)
	}
	providerHTTP := overrides.NotificationHTTP
	if providerHTTP == nil {
		providerHTTP = httpx.NewNamedObserved(httpx.TargetNotifications, httpx.TimeoutNotifications, metricRecorder)
	}
	providerAdapters, providerValidators := notifications.NewHTTPProviderAdapters(
		providerHTTP, func() string { return settings.AccessPublicURL.Get(set) },
	)
	mqttAdapter := notifications.NewMQTTAdapter(func() string { return settings.AccessPublicURL.Get(set) })
	providerAdapters = append(providerAdapters, mqttAdapter)
	providerValidators = append(providerValidators, mqttAdapter)
	if secrets != nil {
		identity := secrets.WebPushIdentity()
		webPushAdapter := notifications.NewWebPushAdapter(notifications.WebPushIdentity{
			PublicKey: identity.PublicKey, PrivateKey: identity.PrivateKey,
		}, overrides.NotificationHTTP, func() string { return settings.AccessPublicURL.Get(set) })
		providerAdapters = append(providerAdapters, webPushAdapter)
		providerValidators = append(providerValidators, webPushAdapter)
	}
	accountDelivery := buildAccountDelivery(
		st, destinationRepository, providerAdapters, set, invitationService, recoveryService, registry, log,
	)
	providerValidators = append(accountDelivery.validators, providerValidators...)
	scheduler, err := buildScheduler(rootCtx, st, set, registry, emitter, owner, log, metricRecorder)
	if err != nil {
		return operationsBuild{}, err
	}
	var jobService system.JobService
	if scheduler != nil {
		jobService = jobsAdapter{s: scheduler}
	}
	triggerHealth := func(ctx context.Context) {
		if jobService == nil {
			return
		}
		if err := jobService.Trigger(ctx, "system-health"); err != nil {
			log.Warn("current health refresh after settings change could not be queued", "err", err)
		}
	}
	result := operationsBuild{
		backups: backups, restart: restart, bootConfig: bootConfig,
		auth:  authResult,
		guide: buildGuide(st, set, playoutResolver, appliedBackend, metricRecorder),
		settings: buildSettings(
			st, set, desiredSet, secrets, libraryClient, tmdbClient,
			refreshSecretRedactor, readGeneratedSecret, triggerHealth, emitter.SettingsChanged, log,
		),
		notificationDestinations: buildNotificationDestinations(
			destinationRepository, providerValidators, accountDelivery.service,
		),
		productNotifications: &productNotificationCoordinator{publisher: accountDelivery.product, source: st, log: log},
		invitationDelivery:   accountDelivery.invitations,
		passwordRecovery:     accountDelivery.recovery,
		jobs:                 jobService,
		scheduler:            scheduler,
		database:             buildDatabase(st, set, overrides, eventBus),
		residentLLM:          buildResidentLLM(set, log),
	}
	if secrets != nil {
		result.webPushPublicKey = secrets.WebPushIdentity().PublicKey
	}
	if st != nil {
		result.liveSettings = set
		result.libraryConfigured = set.libraryConfigured
	}
	return result, nil
}

func migrateLegacySMTPProvider(
	ctx context.Context,
	st store.Store,
	repository notifications.Repository,
	set resolved,
) error {
	if st == nil || repository == nil || set.svc == nil {
		return nil
	}
	const marker = "migration.notifications.smtp_provider.v1"
	return st.WithSettingLock(ctx, marker, func(ctx context.Context) error {
		if _, err := st.GetSetting(ctx, marker); err == nil {
			return nil
		} else if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		existing, err := repository.ListNotificationDestinationMetadata(ctx)
		if err != nil {
			return err
		}
		for _, destination := range existing {
			if destination.Means == notifications.MeansEmail {
				return st.SetSetting(ctx, marker, "true")
			}
		}
		config, err := set.legacyEmailConfig()
		if err != nil {
			return err
		}
		if strings.TrimSpace(config.Host) != "" || strings.TrimSpace(config.FromAddress) != "" ||
			config.Username != "" || config.Password != "" {
			now := time.Now().UTC().Truncate(time.Second)
			if err := repository.SaveNotificationDestination(ctx, notifications.Destination{
				ID: "smtp-legacy", Means: notifications.MeansEmail, Label: "SMTP",
				Scope: notifications.ScopeInstallation, Audience: notifications.RecipientOperators,
				Enabled: config.Enabled,
				Configuration: map[string]string{
					"host": config.Host, "port": strconv.Itoa(config.Port), "security": string(config.Security),
					"fromAddress": config.FromAddress, "fromName": config.FromName, "username": config.Username,
				},
				Credentials: map[string]string{"password": config.Password}, CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				return err
			}
		}
		return st.SetSetting(ctx, marker, "true")
	})
}

func buildNotificationDestinations(
	repository notifications.Repository,
	validators []notifications.DestinationValidator,
	tester notifications.DestinationTester,
) setupapi.NotificationDestinationService {
	if repository == nil {
		return nil
	}
	return notifications.NewDestinationManager(repository, validators, newID, time.Now).WithTester(tester)
}

func buildResidentLLM(set resolved, log *slog.Logger) residentLLMBuild {
	probe := func(ctx context.Context) (float64, string) {
		if settings.LLMProvider.Get(set) != "ollama" || settings.LLMURL.Get(set) == "" {
			return 0, ""
		}
		resident, err := llm.NewOllama(settings.LLMURL.Get(set), settings.LLMModel.Get(set)).ListResident(ctx)
		if err != nil {
			log.Debug("playout doctor: /api/ps residency probe failed", "err", err)
			return 0, ""
		}
		var total float64
		var largest llm.ResidentModel
		for _, model := range resident {
			total += model.VRAMGiB
			if model.VRAMGiB > largest.VRAMGiB {
				largest = model
			}
		}
		return total, largest.Name
	}
	return residentLLMBuild{probe: probe}
}

func buildBackups(st store.Store, set resolved, registry *jobs.Registry, log *slog.Logger) system.BackupsService {
	if writer := store.BackupWriter(st); writer != nil {
		service := &backupsService{
			w: writer, dir: func() string { return settings.BackupDir.Get(set) },
			retain: func() int { return settings.BackupRetain.Get(set) },
			sched:  func() string { return settings.BackupSchedule.Get(set) },
		}
		registry.Add(service.Job(log))
		return service
	}
	if st != nil {
		registry.Add(unavailableBackupJob(
			"Loomarr does not back up PostgreSQL itself — use pg_dump on your usual schedule."))
	}
	return nil
}

type authBuild struct {
	backup               system.BackupStreamer
	authorizer           httpcore.Authorizer
	login                people.LoginService
	sso                  people.SSOService
	sessions             people.SessionManager
	userSync             people.UserSyncer
	provision            people.Provisioner
	password             people.PasswordService
	invitations          people.InvitationService
	invitationRedemption people.InvitationRedemptionService
	passwordRecovery     *auth.PasswordRecoveryService
	deviceManager        *auth.DeviceManager
	deviceLimiter        *auth.RateLimiter
	playoutSecret        func() string
	playoutSecretCurrent func(context.Context) (string, error)
}

func buildAuth(
	st store.Store,
	set resolved,
	secrets *settings.Secrets,
	readGeneratedSecret func(context.Context, settings.GeneratedSecret) (string, error),
	libraryClient *library.Client,
	log *slog.Logger,
) authBuild {
	var result authBuild
	apiToken := ""
	if secrets != nil {
		apiToken = secrets.Value(settings.SecretAPI)
		result.playoutSecret = func() string { return secrets.Value(settings.SecretPlayout) }
		result.playoutSecretCurrent = func(ctx context.Context) (string, error) {
			return readGeneratedSecret(ctx, settings.SecretPlayout)
		}
	}
	result.authorizer = httpcore.NewTokenAuthorizer(apiToken)
	if st == nil {
		return result
	}

	manager := auth.NewManager(st, settings.SessionTTL.Get(set), time.Now)
	loginLimiter := auth.NewRateLimiter(0.2, 5)
	result.login = auth.NewLoginService(libraryClient, st, manager, loginLimiter, time.Now)
	result.userSync = auth.NewUserSync(libraryClient, st, time.Now)
	result.provision = auth.NewProvisioner(st, libraryClient, newID, time.Now)
	result.sso = auth.NewSSOService(func() auth.SSOConfig {
		base := strings.TrimRight(settings.ServerPublicURL.Get(set), "/")
		redirect := ""
		if base != "" {
			redirect = base + "/v1/auth/sso/callback"
		}
		return auth.SSOConfig{
			Enabled: settings.AuthSSOEnabled.Get(set), Issuer: settings.AuthSSOIssuer.Get(set),
			ClientID: settings.AuthSSOClientID.Get(set), ClientSecret: settings.AuthSSOClientSecret.Get(set),
			RedirectURL: redirect,
		}
	}, st, manager, time.Now, log)
	result.password = auth.NewPasswordService(st, newID, time.Now)
	var invitationLibrary access.LibraryAccountResolver
	if libraryClient != nil {
		invitationLibrary = invitationLibraryResolver{client: libraryClient}
	}
	result.invitations = access.NewService(st, invitationLibrary, newID, nil, time.Now)
	result.invitationRedemption = auth.NewInvitationRedemptionService(
		st, libraryClient, manager, newID, time.Now,
	)
	result.passwordRecovery = auth.NewPasswordRecoveryService(st, newID, nil, time.Now)
	result.sessions = manager
	result.deviceManager = auth.NewDeviceManager(st, time.Now)
	result.deviceLimiter = auth.NewRateLimiter(0.05, 5)
	result.authorizer = api.NewSessionAuthorizerCurrent(
		manager, result.deviceManager,
		func(ctx context.Context) (string, error) {
			return readGeneratedSecret(ctx, settings.SecretAPI)
		},
	)
	if backuper := store.SQLiteBackuper(st); backuper != nil {
		result.backup = backuper
	}
	return result
}

func buildGuide(
	st store.Store,
	set resolved,
	playoutResolver *playoutresolve.Resolver,
	appliedBackend func(context.Context) (string, error),
	metricRecorder *metrics.Recorder,
) guideapi.GuideReader {
	if st == nil {
		return nil
	}
	var internalGuide nownext.BroadcastReader
	if playoutResolver != nil {
		internalGuide = playoutResolver
	}
	return guideReader{router: nownext.New(nownext.Config{
		Tunarr: guideAdapter{
			tunarr: tunarr.NewDynamicObserved(set.tunarrConfig(), metricRecorder), window: 2 * time.Hour,
		},
		Internal: internalGuide, Channels: st, AppliedBackend: appliedBackend, Window: 2 * time.Hour,
	})}
}

func buildSettings(
	st store.Store,
	set resolved,
	desiredSet resolved,
	secrets *settings.Secrets,
	libraryClient *library.Client,
	tmdbClient *tmdb.Client,
	refreshRedactor func(),
	readSecret func(context.Context, settings.GeneratedSecret) (string, error),
	triggerHealth func(context.Context),
	settingsChanged func(),
	log *slog.Logger,
) setupapi.SettingsService {
	if st == nil || secrets == nil {
		return nil
	}
	return settingsAdapter{
		svc: set.svc, secrets: secrets, store: st, log: log, protection: set.protection,
		tests: connectionTests(desiredSet, libraryClient, tmdbClient), refreshRedactor: refreshRedactor,
		readSecret: readSecret, triggerHealth: triggerHealth, changed: settingsChanged,
	}
}

func buildScheduler(
	rootCtx context.Context,
	st store.Store,
	set resolved,
	registry *jobs.Registry,
	emitter *eventEmitter,
	owner *generationLifecycle,
	log *slog.Logger,
	metricRecorder *metrics.Recorder,
) (*jobs.Scheduler, error) {
	if st == nil {
		return nil, nil
	}
	service := jobs.New(st, registry, func(key, fallback string) string {
		// Job schedule keys arrive from the jobs registry, so the key is dynamic; the answer
		// is a string setting or the caller's fallback.
		if configured, _ := set.Value(key).(string); configured != "" {
			return configured
		}
		return fallback
	}, time.Now, log).WithNotifier(emitter).WithObserver(metricRecorder)
	service.SeedRegistry(rootCtx)
	// River is the only job executor: without it nothing runs on its schedule, so a failure to
	// start fails the build rather than serving an app with a dead background.
	pool := store.PoolOf(st)
	if pool == nil {
		return nil, errors.New("start River job queue: store exposes no database pool")
	}
	stop, err := service.StartRiver(rootCtx, store.DialectOf(st), pool, log)
	if err != nil {
		return nil, fmt.Errorf("start River job queue: %w", err)
	}
	owner.addStop(stop)
	return service, nil
}
