package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/auth"
	"github.com/loomarr/loomarr/internal/config"
	"github.com/loomarr/loomarr/internal/events"
	"github.com/loomarr/loomarr/internal/httpx"
	"github.com/loomarr/loomarr/internal/invitation"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/metrics"
	"github.com/loomarr/loomarr/internal/notifications"
	"github.com/loomarr/loomarr/internal/programmer"
	"github.com/loomarr/loomarr/internal/scheduler"
	"github.com/loomarr/loomarr/internal/secretprotection"
	"github.com/loomarr/loomarr/internal/settings"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/tmdb"
)

func buildRestart(overrides Overrides, log *slog.Logger) (api.RestartService, *config.Config) {
	var service api.RestartService
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

func buildDatabase(st store.Store, set resolved, overrides Overrides, eventBus *events.Bus) api.DatabaseService {
	if st == nil {
		return nil
	}
	dataDir := ""
	if store.DialectOf(st) == store.DialectSQLite {
		dataDir = filepath.Dir(store.SQLitePath(st))
	}
	return newDatabaseService(st, dataDir, func() string { return set.str("backup.dir") }, eventBus).
		WithMigrationRequest(overrides.DatabaseMigration).
		WithLastError(overrides.DatabaseMigrationError)
}

type residentLLMBuild struct {
	probe   func(context.Context) (float64, string)
	reclaim func(context.Context)
}

type operationsBuild struct {
	backups                  api.BackupsService
	restart                  api.RestartService
	bootConfig               *config.Config
	auth                     authBuild
	guide                    api.GuideReader
	settings                 api.SettingsService
	emailTest                api.EmailTestService
	notificationDestinations api.NotificationDestinationService
	webPushPublicKey         string
	productNotifications     *productNotificationCoordinator
	invitationDelivery       api.InvitationDeliveryService
	passwordRecovery         api.PasswordRecoveryService
	liveConfig               func(string) string
	libraryConfigured        func() bool
	jobs                     api.JobService
	database                 api.DatabaseService
	residentLLM              residentLLMBuild
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
	registry *scheduler.Registry,
	owner *generationLifecycle,
	playoutResolver *playoutResolver,
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
	invitationService, _ := authResult.invitations.(*invitation.Service)
	recoveryService := authResult.passwordRecovery
	destinationRepository := notifications.NewProtectedDestinationRepository(st, protection).
		WithCredentialRedactor(secretRedactor)
	if destinationRepository != nil {
		if err := destinationRepository.SeedCredentialRedaction(rootCtx); err != nil {
			return operationsBuild{}, fmt.Errorf("register notification provider credentials for redaction: %w", err)
		}
	}
	if err := migrateLegacySMTPProvider(rootCtx, destinationRepository, set); err != nil {
		return operationsBuild{}, fmt.Errorf("migrate SMTP notification provider: %w", err)
	}
	providerHTTP := overrides.NotificationHTTP
	if providerHTTP == nil {
		providerHTTP = httpx.NewNamedObserved("notification-provider", httpx.TimeoutNotifications, metricRecorder)
	}
	providerAdapters, providerValidators := notifications.NewHTTPProviderAdapters(
		providerHTTP, func() string { return set.str("access.public_url") },
	)
	mqttAdapter := notifications.NewMQTTAdapter(func() string { return set.str("access.public_url") })
	providerAdapters = append(providerAdapters, mqttAdapter)
	providerValidators = append(providerValidators, mqttAdapter)
	if secrets != nil {
		identity := secrets.WebPushIdentity()
		webPushAdapter := notifications.NewWebPushAdapter(notifications.WebPushIdentity{
			PublicKey: identity.PublicKey, PrivateKey: identity.PrivateKey,
		}, overrides.NotificationHTTP, func() string { return set.str("access.public_url") })
		providerAdapters = append(providerAdapters, webPushAdapter)
		providerValidators = append(providerValidators, webPushAdapter)
	}
	accountDelivery := buildAccountDelivery(
		st, destinationRepository, providerAdapters, set, invitationService, recoveryService, registry, log,
	)
	providerValidators = append(accountDelivery.validators, providerValidators...)
	jobs := buildScheduler(rootCtx, st, set, registry, emitter, owner, log, metricRecorder)
	triggerHealth := func(ctx context.Context) {
		if jobs == nil {
			return
		}
		if err := jobs.Trigger(ctx, "system-health"); err != nil {
			log.Warn("current health refresh after settings change could not be queued", "err", err)
		}
	}
	result := operationsBuild{
		backups: backups, restart: restart, bootConfig: bootConfig,
		auth:  authResult,
		guide: buildGuide(st, set, playoutResolver, appliedBackend, metricRecorder),
		settings: buildSettings(
			st, set, desiredSet, secrets, libraryClient, tmdbClient,
			refreshSecretRedactor, readGeneratedSecret, triggerHealth, log,
		),
		emailTest: buildEmailTest(st, set),
		notificationDestinations: buildNotificationDestinations(
			destinationRepository, providerValidators, accountDelivery.service,
		),
		productNotifications: &productNotificationCoordinator{publisher: accountDelivery.product, source: st, log: log},
		invitationDelivery:   accountDelivery.invitations,
		passwordRecovery:     accountDelivery.recovery,
		jobs:                 jobs,
		database:             buildDatabase(st, set, overrides, eventBus),
		residentLLM:          buildResidentLLM(set, log),
	}
	if secrets != nil {
		result.webPushPublicKey = secrets.WebPushIdentity().PublicKey
	}
	if st != nil {
		result.liveConfig = set.str
		result.libraryConfigured = set.libraryConfigured
	}
	return result, nil
}

func migrateLegacySMTPProvider(
	ctx context.Context,
	repository notifications.DestinationRepository,
	set resolved,
) error {
	if repository == nil || set.svc == nil {
		return nil
	}
	existing, err := repository.ListNotificationDestinationMetadata(ctx)
	if err != nil {
		return err
	}
	for _, destination := range existing {
		if destination.Means == notifications.MeansEmail {
			return nil
		}
	}
	config := set.emailConfig()
	if strings.TrimSpace(config.Host) == "" && strings.TrimSpace(config.FromAddress) == "" &&
		config.Username == "" && config.Password == "" {
		return nil
	}
	now := time.Now().UTC().Truncate(time.Second)
	return repository.SaveNotificationDestination(ctx, notifications.Destination{
		ID: "smtp-legacy", Means: notifications.MeansEmail, Label: "SMTP",
		Scope: notifications.ScopeInstallation, Audience: notifications.RecipientOperators,
		Enabled: config.Enabled,
		Configuration: map[string]string{
			"host": config.Host, "port": strconv.Itoa(config.Port), "security": string(config.Security),
			"fromAddress": config.FromAddress, "fromName": config.FromName, "username": config.Username,
		},
		Credentials: map[string]string{"password": config.Password}, CreatedAt: now, UpdatedAt: now,
	})
}

func buildNotificationDestinations(
	repository notifications.DestinationRepository,
	validators []notifications.DestinationValidator,
	tester notifications.DestinationTester,
) api.NotificationDestinationService {
	if repository == nil {
		return nil
	}
	return notifications.NewDestinationManager(repository, validators, newID, time.Now).WithTester(tester)
}

func buildEmailTest(st store.Store, set resolved) api.EmailTestService {
	if st == nil || set.svc == nil {
		return nil
	}
	adapter := notifications.NewEmailAdapter(set.emailConfig, nil, notifications.NewSMTPSender(15*time.Second))
	return emailTestAdapter{adapter: adapter}
}

func buildResidentLLM(set resolved, log *slog.Logger) residentLLMBuild {
	probe := func(ctx context.Context) (float64, string) {
		if set.str("llm.provider") != "ollama" || set.str("llm.url") == "" {
			return 0, ""
		}
		resident, err := llm.NewOllama(set.str("llm.url"), set.str("llm.model")).ListResident(ctx)
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
	reclaim := func(ctx context.Context) {
		if set.str("llm.provider") != "ollama" || set.str("llm.url") == "" {
			return
		}
		provider := llm.NewProvider("ollama", set.str("llm.url"), set.str("llm.model"), "")
		evictor, ok := provider.(llm.Evictor)
		if !ok {
			return
		}
		if err := evictor.Evict(ctx); err != nil && !errors.Is(err, llm.ErrNothingToEvict) {
			log.Debug("playout: LLM eviction failed (encode will fall back to software)", "err", err)
		}
	}
	return residentLLMBuild{probe: probe, reclaim: reclaim}
}

func buildBackups(st store.Store, set resolved, jobs *scheduler.Registry, log *slog.Logger) api.BackupsService {
	if writer := store.BackupWriter(st); writer != nil {
		service := &backupsService{
			w: writer, dir: func() string { return set.str("backup.dir") },
			retain: func() int { return set.intv("backup.retain") },
			sched:  func() string { return set.str("backup.schedule") },
		}
		jobs.Add(service.Job(log))
		return service
	}
	if st != nil {
		jobs.Add(unavailableBackupJob(
			"Loomarr does not back up PostgreSQL itself — use pg_dump on your usual schedule."))
	}
	return nil
}

type authBuild struct {
	backup               api.BackupStreamer
	authorizer           api.Authorizer
	login                api.LoginService
	sso                  api.SSOService
	sessions             api.SessionManager
	userSync             api.UserSyncer
	provision            api.Provisioner
	password             api.PasswordService
	invitations          api.InvitationService
	invitationRedemption api.InvitationRedemptionService
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
	result.authorizer = api.NewTokenAuthorizer(apiToken)
	if st == nil {
		return result
	}

	manager := auth.NewManager(st, set.dur("session.ttl"), time.Now)
	loginLimiter := auth.NewRateLimiter(0.2, 5)
	result.login = auth.NewLoginService(libraryClient, st, manager, loginLimiter, time.Now)
	result.userSync = auth.NewUserSync(libraryClient, st, time.Now)
	result.provision = auth.NewProvisioner(st, libraryClient, newID, time.Now)
	result.sso = auth.NewSSOService(func() auth.SSOConfig {
		base := strings.TrimRight(set.str("server.public_url"), "/")
		redirect := ""
		if base != "" {
			redirect = base + "/v1/auth/sso/callback"
		}
		return auth.SSOConfig{
			Enabled: set.boolv("auth.sso.enabled"), Issuer: set.str("auth.sso.issuer"),
			ClientID: set.str("auth.sso.client_id"), ClientSecret: set.str("auth.sso.client_secret"),
			RedirectURL: redirect,
		}
	}, st, manager, time.Now, log)
	result.password = auth.NewPasswordService(st, newID, time.Now)
	var invitationLibrary invitation.LibraryAccountResolver
	if libraryClient != nil {
		invitationLibrary = invitationLibraryResolver{client: libraryClient}
	}
	result.invitations = invitation.NewService(st, invitationLibrary, newID, nil, time.Now)
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
	playoutResolver *playoutResolver,
	appliedBackend func(context.Context) (string, error),
	metricRecorder *metrics.Recorder,
) api.GuideReader {
	if st == nil {
		return nil
	}
	var internalGuide broadcastReader
	if playoutResolver != nil {
		internalGuide = playoutResolver
	}
	return nowNextRouter{
		tunarr: guideAdapter{
			tunarr: programmer.NewDynamicObserved(set.tunarrConfig(), metricRecorder), window: 2 * time.Hour,
		},
		internal: internalGuide, channels: st, appliedBackend: appliedBackend, window: 2 * time.Hour,
	}
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
	log *slog.Logger,
) api.SettingsService {
	if st == nil || secrets == nil {
		return nil
	}
	return settingsAdapter{
		svc: set.svc, secrets: secrets, store: st, log: log, protection: set.protection,
		tests: connectionTests(desiredSet, libraryClient, tmdbClient), refreshRedactor: refreshRedactor,
		readSecret: readSecret, triggerHealth: triggerHealth,
	}
}

func buildScheduler(
	rootCtx context.Context,
	st store.Store,
	set resolved,
	registry *scheduler.Registry,
	emitter *eventEmitter,
	owner *generationLifecycle,
	log *slog.Logger,
	metricRecorder *metrics.Recorder,
) api.JobService {
	if st == nil {
		return nil
	}
	service := scheduler.New(st, registry, func(key, fallback string) string {
		if configured := set.str(key); configured != "" {
			return configured
		}
		return fallback
	}, time.Now, log).WithNotifier(emitter).WithObserver(metricRecorder)
	service.SeedRegistry(rootCtx)
	if pool := store.PoolOf(st); pool != nil {
		stop, err := service.StartRiver(rootCtx, st, pool, log)
		if err != nil {
			log.Error("scheduler: River did not start — no job will run on its schedule", "err", err)
		} else {
			owner.addStop(stop)
		}
	} else {
		log.Error("scheduler: store exposes no pool — no job will run on its schedule")
	}
	return jobsAdapter{s: service}
}
