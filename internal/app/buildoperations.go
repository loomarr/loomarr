package app

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/auth"
	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/programmer"
	"github.com/mantonx/loomarr/internal/scheduler"
	"github.com/mantonx/loomarr/internal/settings"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/tmdb"
)

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
			tunarr: programmer.NewDynamic(set.tunarrConfig()), window: 2 * time.Hour,
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
	log *slog.Logger,
) api.SettingsService {
	if st == nil || secrets == nil {
		return nil
	}
	return settingsAdapter{
		svc: set.svc, secrets: secrets, store: st, log: log,
		tests: connectionTests(desiredSet, libraryClient, tmdbClient), refreshRedactor: refreshRedactor,
		readSecret: readSecret,
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
) api.JobService {
	if st == nil {
		return nil
	}
	service := scheduler.New(st, registry, func(key, fallback string) string {
		if configured := set.str(key); configured != "" {
			return configured
		}
		return fallback
	}, time.Now, log).WithNotifier(emitter)
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
