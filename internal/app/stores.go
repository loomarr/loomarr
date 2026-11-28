package app

import (
	"context"
	"database/sql"
	"io"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/channels/approval"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/library/resolvers"
	"github.com/loomarr/loomarr/internal/platform/holidayvocab"
	"github.com/loomarr/loomarr/internal/platform/housekeeping"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
	"github.com/loomarr/loomarr/internal/playout/resolve"
	"github.com/loomarr/loomarr/internal/proposals/accept"
	"github.com/loomarr/loomarr/internal/proposals/quality"
)

// A builder takes a struct of the roles it hands on, not the aggregate store.Store: the struct
// says what the builder can reach. Each is filled from the one store Build holds; a nil store
// leaves every role nil, and the builder skips what needs persistence.

// A builder that hands the store to many consumers takes one interface naming their roles.

// playoutStore is what playout, its backend transition and its lifecycle reach.
type playoutStore interface {
	store.SettingStore
	store.ChannelStore
	resolve.TitleReader
	resolve.ClipPlayRecorder
	resolve.AiringRecorder
	resolve.ChannelReader
	resolve.CodecWriter
	resolve.LibraryAudioReader
	resolve.LibraryFileReader
	resolve.LibraryVideoReader
	resolve.LibrarySubtitleReader
	librarystore.LibraryItemFactsReader
	inventory.Repository
	inventory.AnalysisStore
	upcomingWriter
}

// channelsStore adds what the channel engine and its sweep reach.
type channelsStore interface {
	playoutStore
	channels.EngineStore
	channels.AvailabilityStore
	quality.ObservationRecorder
	librarystore.LibraryEpisodeReader
	librarystore.LibraryTitleReader
	resolvers.OwnCollections
}

// operationsStore is what settings, sign-in, jobs, the guide and notifications reach.
type operationsStore interface {
	store.SettingStore
	store.UserStore
	store.ChannelStore
	store.TitleStore
	store.ScheduledJobStore
	store.InvitationStore
	store.PasswordRecoveryStore
	store.NotificationStore
}

// foundationStore is what boot reaches: settings, secrets, diagnostics and the language model.
type foundationStore interface {
	store.CountStore
	store.TableRowStore
	store.InteractiveOperationStore
	holidayvocab.Lister
	store.SettingStore
	store.SecretProtectionStore
	store.DiagnosticStore
	store.ClientCrashStore
	store.ActivityStore
	store.NotificationStore
}

// artworkStore is what the image service's owners and jobs read and write.
type artworkStore interface {
	clipstore.ImageStore
	store.ChannelStore
	store.ClipStore
	store.TitleStore
	store.UserStore
}

// watermarkStore is what the channel bug reads.
type watermarkStore interface {
	store.ChannelStore
	store.SettingStore
	clipstore.ImageStore
}

// podStore is what pod assembly reads.
type podStore interface {
	store.FillerBreakStore
	store.ClipStore
	store.AiringStore
}

// databaseHandle is what boot and operations need from the database itself rather than from a role.
type databaseHandle struct {
	// open is false without a store; the rest are then zero.
	open    bool
	dialect storedb.Dialect
	// writer writes a backup file; nil for a backend Loomarr does not back up.
	writer interface {
		WriteBackup(ctx context.Context, dir string) (store.BackupFile, error)
	}
	// streamer streams a SQLite backup to a download; nil otherwise.
	streamer interface {
		StreamBackup(ctx context.Context, w io.Writer) error
	}
	// pool is the connection pool the job queue runs on.
	pool *sql.DB
	// sqlitePath is the database file, empty for a backend with none.
	sqlitePath string
}

func databaseOf(st store.Store) databaseHandle {
	if st == nil {
		return databaseHandle{}
	}
	return databaseHandle{
		open: true, dialect: store.DialectOf(st), writer: store.BackupWriter(st),
		streamer: store.SQLiteBackuper(st), pool: store.PoolOf(st), sqlitePath: store.SQLitePath(st),
	}
}

// invalidationHandle is the whole store, which only the PostgreSQL invalidation listener needs: it
// opens its own connection from the store's handle.
type invalidationHandle = store.Store

// invalidationsOf is the handle for a backend that publishes invalidations, nil otherwise.
func invalidationsOf(st store.Store) invalidationHandle {
	if st == nil || !store.PublishesInvalidations(store.DialectOf(st)) {
		return nil
	}
	return st
}

type approvalStores struct {
	binder       approval.Store
	approver     accept.ApproveStore
	observations quality.ObservationRecorder
	library      librarystore.LibraryTitleReader
}

func newApprovalStores(st store.Store) approvalStores {
	if st == nil {
		return approvalStores{}
	}
	return approvalStores{binder: st, approver: st, observations: st, library: st}
}

type provisioningStores struct {
	titles       store.TitleStore
	library      librarystore.LibraryTitleReader
	observations quality.ObservationRecorder
	housekeeping housekeeping.Store
	// tidy compacts the database; nil for a backend that needs none.
	tidy func(context.Context) (freedBytes int64, err error)
}

// Housekeeping purges filler records too, so this one is filled from the filler extension.
func newProvisioningStores(st records.Combined) provisioningStores {
	if st == nil {
		return provisioningStores{}
	}
	roles := provisioningStores{titles: st, library: st, observations: st, housekeeping: st}
	if tidier := store.DatabaseTidier(st); tidier != nil {
		roles.tidy = tidier.Tidy
	}
	return roles
}
