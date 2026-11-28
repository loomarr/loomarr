package store

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/platform/diagnostics"
)

// ClientCrashStore keeps crash reports from paired apps (#1816). The diagnostics module validates
// and scrubs a report before it gets here; the store only persists, pages and purges. The
// implementation is internal/platform/store/crashstore.
type ClientCrashStore interface {
	InsertClientCrashReport(ctx context.Context, record diagnostics.ClientCrashRecord) error
	// QueryClientCrashReports pages newest first and leaves the heavy columns (stack,
	// breadcrumbs, native payload) empty.
	QueryClientCrashReports(ctx context.Context, query diagnostics.ClientCrashStoreQuery) ([]diagnostics.ClientCrashRecord, error)
	GetClientCrashReport(ctx context.Context, id string) (diagnostics.ClientCrashRecord, bool, error)
	// PurgeClientCrashReports removes reports received before `before`, then all but the newest
	// `keep`, and reports how many it removed.
	PurgeClientCrashReports(ctx context.Context, before time.Time, keep int) (int, error)
}
