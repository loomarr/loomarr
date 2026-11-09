package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	guideapi "github.com/loomarr/loomarr/internal/api/guide"
	"github.com/loomarr/loomarr/internal/api/httpcore"
	opsapi "github.com/loomarr/loomarr/internal/api/ops"
	"github.com/loomarr/loomarr/internal/api/people"
	proposalsapi "github.com/loomarr/loomarr/internal/api/proposals"
	setupapi "github.com/loomarr/loomarr/internal/api/setup"
	"github.com/loomarr/loomarr/internal/api/system"
	"github.com/loomarr/loomarr/internal/app/playoutresolve"
	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/channels/viewing"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/platform/config"
	"github.com/loomarr/loomarr/internal/platform/diagnostics"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/proposals/outlook"
)

type httpBuild struct {
	rootCtx     context.Context
	store       records.Store
	log         *slog.Logger
	overrides   Overrides
	foundation  foundationBuild
	channels    channelBuild
	approval    approvalBuild
	suggestions suggestionBuild
	fillers     fillerBuild
	auth        authBuild
	backups     system.BackupsService
	// watermarks draws the channel page's On-screen logo preview (#1659).
	watermarks               *channelWatermarks
	restart                  system.RestartService
	bootConfig               *config.Config
	guide                    guideapi.GuideReader
	settings                 setupapi.SettingsService
	notificationDestinations setupapi.NotificationDestinationService
	webPushPublicKey         string
	invitationDelivery       people.InvitationDeliveryService
	passwordRecovery         people.PasswordRecoveryService
	liveSettings             settings.Reader
	libraryConfigured        func() bool
	jobs                     system.JobService
	database                 system.DatabaseService
	residentLLM              residentLLMBuild
	// library is Settings → Library, the scanner over its folders, and the arrs' webhooks.
	library libraryServices
}

func buildHTTP(deps httpBuild) http.Handler {
	rootCtx, st, log, ov := deps.rootCtx, deps.store, deps.log, deps.overrides
	set, desiredSet := deps.foundation.set, deps.foundation.desiredSet
	ready, eventBus := deps.foundation.ready, deps.foundation.eventBus
	activityRec, fillerLayout := deps.foundation.activity, deps.foundation.fillerLayout
	appliedRestartSettings := deps.foundation.appliedRestartSettings
	channelSvc, liveTVSvc := deps.channels.channelService, deps.channels.liveTV
	tunerRescanner, tunarrConnectSvc := deps.channels.tunerRescanner, deps.channels.tunarrConnector
	backendController := deps.channels.backendController
	refreshBackendSettings, desiredBackend := deps.channels.refreshBackendSettings, deps.channels.desiredBackend
	checkpointSnapshot := deps.channels.checkpoint
	playoutObserver := deps.channels.playoutObserver
	playoutCapability := deps.channels.playoutCapability
	playoutSvc, playoutResolverSvc := deps.channels.playout, deps.channels.playoutResolverService
	playoutGuideSvc := deps.channels.playoutGuide
	proposalApprover, chBinder := deps.approval.approver, deps.approval.binder
	var proposalOutlook proposalsapi.ProposalOutlook
	if preview, ok := channelSvc.(outlook.Preview); ok && chBinder != nil {
		additionResolver := approvalAdditionAdapter{presence: func() catalog.LibraryPresence {
			return catalog.LibraryOwnership{Titles: st}
		}}
		proposalOutlook = outlook.New(outlook.Config{
			Titles: st, Library: playoutresolve.ItemMetadata{Facts: st}, Episodes: channels.LibraryEpisodeResolver(st),
			Planner: chBinder, Preview: preview, Additions: additionResolver, Holidays: deps.foundation.holidays,
		})
	}
	suggestSvc, proposalWorkflow := deps.suggestions.suggest, deps.suggestions.workflow
	searchSvc, movieCollectionsSvc, collectionsSvc := deps.suggestions.search, deps.suggestions.movieCollections, deps.suggestions.collections
	systemLLM, iconSvc, imageSvc := deps.suggestions.systemLLM, deps.suggestions.icons, deps.suggestions.images
	timelineThumbs := deps.suggestions.timelineThumbs
	fillerSvc, podPreview, taxonomyEditor := deps.fillers.service, deps.fillers.preview, deps.fillers.taxonomy
	backup, authorizer := deps.auth.backup, deps.auth.authorizer
	loginSvc, ssoSvc := deps.auth.login, deps.auth.sso
	sessMgr, userSync := deps.auth.sessions, deps.auth.userSync
	provisionSvc, passwordSvc, invitationSvc := deps.auth.provision, deps.auth.password, deps.auth.invitations
	invitationRedemption := deps.auth.invitationRedemption
	deviceMgr, deviceLimiter := deps.auth.deviceManager, deps.auth.deviceLimiter
	playoutSecret, playoutSecretCurrent := deps.auth.playoutSecret, deps.auth.playoutSecretCurrent
	backupsSvc, restartSvc, bootCfg := deps.backups, deps.restart, deps.bootConfig
	guideSvc, settingsSvc := deps.guide, deps.settings
	liveSettings, libraryConfigured := deps.liveSettings, deps.libraryConfigured
	jobsSvc, databaseSvc, residentLLM := deps.jobs, deps.database, deps.residentLLM
	return api.Router(log, api.Options{
		Store:                st,
		Auth:                 authorizer,
		Log:                  log,
		Metrics:              deps.foundation.metrics,
		BackupSQLite:         backup,
		Ready:                ready,
		Login:                loginSvc,
		Sessions:             sessMgr,
		Passwords:            passwordSvc,
		Invitations:          invitationSvc,
		InvitationRedemption: invitationRedemption,
		InvitationDelivery:   deps.invitationDelivery,
		PasswordRecovery:     deps.passwordRecovery,
		AccessPublicURL:      func() string { return settings.AccessPublicURL.Get(liveSettings) },
		UserSync:             userSync,
		Devices:              deviceMgr,
		DeviceLimiter:        deviceLimiter,
		CookieSecure:         settings.CookieSecure.Get(set),
		TrustProxy:           settings.SecurityTrustProxy.Get(set),
		DevLogin:             ov.DevLogin,
		Pprof:                ov.Pprof,
		MetricsToken:         ov.MetricsToken,
		Demo:                 people.DemoAccount{Username: ov.DemoUsername, Password: ov.DemoPassword},
		Channels:             channelSvc,
		LiveTV:               liveTVSvc,
		TunerRescanner:       tunerRescanner,
		TunarrConnect:        tunarrConnectSvc,
		ArrDiscovery:         arrDiscovery{set: set},
		Suggest:              suggestSvc,
		ProposalWorkflow:     proposalWorkflow,
		ProposalProgress:     deps.suggestions.progress,
		Search:               searchSvc,
		MovieCollections:     movieCollectionsSvc,
		Collections:          collectionsSvc,
		LibraryRoots:         deps.library.roots,
		LibraryHooks:         deps.library.hooks,
		IdeaLibrary:          deps.suggestions.ideaLibrary,
		Icons:                iconSvc,
		Images:               imageService(imageSvc),
		Events:               eventBus,
		Shutdown:             rootCtx.Done(),
		Filler:               fillerSvc,
		SourceProviders:      deps.fillers.providers,
		FillerResearch:       deps.fillers.research,
		IntakeLanguages:      deps.fillers.intakeLanguages,
		FillerDefects:        deps.fillers.defects,
		Pods:                 podPreview,
		Taxonomy:             taxonomyEditor,
		SystemLLM:            systemLLM,
		Database:             databaseSvc,
		Encryption:           buildEncryptionService(st, deps.foundation.protection),
		Backups:              backupsSvc,
		SSO:                  ssoSvc,
		Restart:              restartSvc,
		Activity:             activityRec,
		DiagnosticEvents:     deps.foundation.diagnosticEvents,
		DiagnosticProcesses:  deps.foundation.diagnosticProcesses,
		DiagnosticBundles:    deps.foundation.diagnosticBundles,
		DiagnosticCapture:    deps.foundation.diagnostics,
		ClientDiagnostics:    deps.foundation.clientDiagnostics,
		ClientCrashes:        clientCrashService(deps.foundation.clientCrashes),
		StartupReports:       deps.foundation.startupReports,
		// The baseline for "has a restart-scoped setting changed?" is what THIS
		// generation booted with, captured here rather than per call (config-design §3).
		RestartDrift:             restartDrift(bootCfg, appliedRestartSettings, canonicalRestartCurrent(desiredSet)),
		Jobs:                     jobsSvc,
		Settings:                 settingsSvc,
		NotificationDestinations: deps.notificationDestinations,
		WebPushPublicKey:         deps.webPushPublicKey,
		ProposalNotifications:    deps.foundation.emitter,
		DecisionQuality:          deps.approval.decisionQuality,
		BackendTransition: currentBackendTransition{
			controller: backendController, refresh: refreshBackendSettings, desired: desiredBackend,
		},
		Guide:             guideSvc,
		Provision:         provisionSvc,
		ProposalOutlook:   proposalOutlook,
		Approver:          proposalApprover,
		Binder:            chBinder,
		FillerLayout:      fillerLayout,
		LiveSettings:      liveSettings,
		LibraryConfigured: libraryConfigured,
		BackendCheckpoint: func(ctx context.Context) (httpcore.BackendCheckpoint, error) {
			snapshot, err := checkpointSnapshot(ctx)
			return httpcore.BackendCheckpoint{
				Applied: snapshot.Applied, Prepared: snapshot.Prepared,
				PublishedInternal: snapshot.PublishedInternal,
			}, err
		},
		// Internal playout (§9.1). PlayoutSecret is a FUNC so a regenerated token takes
		// effect without a restart (§11 rotation).
		PlayoutObserver:   playoutObserver,
		Viewing:           startWatchHistory(rootCtx, st, guideSvc, log),
		PlayoutCapability: playoutCapability,
		WatermarkPreview:  deps.watermarks,
		// The in-app HLS repackager for the Watch surface (§9.1, V46). Nil ⇒ /playout/hls 501s.
		Playout:         playoutSvc,
		PlayoutResolver: playoutResolverSvc,
		// The XMLTV guide reads the same resolver, so listings cannot drift from playout.
		PlayoutGuide:         playoutGuideSvc,
		TimelineThumbs:       timelineThumbs,
		PlayoutSecret:        playoutSecret,
		PlayoutSecretCurrent: playoutSecretCurrent,
		// The doctor's TRUE resident-VRAM reading (§9.1 V47), extracted to residentLLMVRAMFn above so
		// the admission budget's VRAM shading (V49) shares the exact same source.
		ResidentLLMVRAM: residentLLM.probe,
	})
}

// startWatchHistory makes the household-viewing tracker the API's playlist polls feed and starts
// the recorder that writes it to the watch history (#3010) until ctx ends. Each row's programme is
// the title the guide says is airing; a gap between programmes has none.
func startWatchHistory(ctx context.Context, st records.Store, guide guideapi.GuideReader, log *slog.Logger) *viewing.Tracker {
	tracker := viewing.New()
	var programmes viewing.Programmes
	if guide != nil {
		programmes = func(ctx context.Context, at time.Time) (map[string]string, error) {
			byChannel, err := guide.NowNext(ctx, at)
			if err != nil {
				return nil, err
			}
			titles := make(map[string]string, len(byChannel))
			for id, nn := range byChannel {
				if nn.Now != nil && !nn.Now.Gap {
					titles[id] = nn.Now.Title
				}
			}
			return titles, nil
		}
	}
	go viewing.NewRecorder(tracker, st, programmes, log).Run(ctx)
	return tracker
}

// clientCrashService keeps a store-less build's nil module from becoming a non-nil interface,
// which would mount the crash routes over nothing.
func clientCrashService(c *diagnostics.ClientCrashes) opsapi.ClientCrashService {
	if c == nil {
		return nil
	}
	return c
}
