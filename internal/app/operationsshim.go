package app

import (
	"context"

	opsapi "github.com/loomarr/loomarr/internal/api/ops"
	"github.com/loomarr/loomarr/internal/api/system"
	"github.com/loomarr/loomarr/internal/platform/backup"
	"github.com/loomarr/loomarr/internal/platform/databasemigration"
	"github.com/loomarr/loomarr/internal/platform/events"
)

// backupsAPI implements system.BackupsService over the backups service, mapping its views to the
// API's.
type backupsAPI struct{ service *backup.Service }

func (b backupsAPI) List(ctx context.Context) (system.BackupList, error) {
	list, err := b.service.List(ctx)
	if err != nil {
		return system.BackupList{}, err
	}
	out := system.BackupList{
		Supported: list.Supported, Dir: list.Dir, Schedule: list.Schedule, Retain: list.Retain,
		Backups: make([]system.BackupEntry, len(list.Backups)),
	}
	for i, e := range list.Backups {
		out.Backups[i] = system.BackupEntry(e)
	}
	return out, nil
}

func (b backupsAPI) Open(ctx context.Context, name string) (system.BackupContent, error) {
	content, err := b.service.Open(ctx, name)
	if err != nil {
		return system.BackupContent{}, err
	}
	return system.BackupContent(content), nil
}

func (b backupsAPI) Run(ctx context.Context) (system.BackupEntry, error) {
	entry, err := b.service.Run(ctx)
	if err != nil {
		return system.BackupEntry{}, err
	}
	return system.BackupEntry(entry), nil
}

// databaseAPI implements system.DatabaseService over the migration service, mapping its views to
// the API's.
type databaseAPI struct{ service *databasemigration.Service }

func (d databaseAPI) Status(ctx context.Context) (system.DatabaseStatus, error) {
	st, err := d.service.Status(ctx)
	if err != nil {
		return system.DatabaseStatus{}, err
	}
	return toDatabaseStatus(st), nil
}

func (d databaseAPI) Preflight(ctx context.Context, dsn string) ([]system.DatabaseCheck, error) {
	checks, err := d.service.Preflight(ctx, dsn)
	if err != nil {
		return nil, err
	}
	out := make([]system.DatabaseCheck, len(checks))
	for i, c := range checks {
		out[i] = system.DatabaseCheck(c)
	}
	return out, nil
}

func (d databaseAPI) Backup(ctx context.Context) (system.DatabaseBackup, error) {
	bk, err := d.service.Backup(ctx)
	if err != nil {
		return system.DatabaseBackup{}, err
	}
	return system.DatabaseBackup(bk), nil
}

func (d databaseAPI) Migrate(ctx context.Context, dsn string) error {
	return d.service.Migrate(ctx, dsn)
}

func toDatabaseStatus(st databasemigration.Status) system.DatabaseStatus {
	out := system.DatabaseStatus{
		Backend: st.Backend, CanMigrate: st.CanMigrate, Phase: st.Phase,
		Tables: toDatabaseTables(st.Tables), Parity: st.Parity, Error: st.Error,
	}
	if st.Backup != nil {
		bk := system.DatabaseBackup(*st.Backup)
		out.Backup = &bk
	}
	return out
}

func toDatabaseTables(tables []databasemigration.Table) []system.DatabaseTable {
	if tables == nil {
		return nil
	}
	out := make([]system.DatabaseTable, len(tables))
	for i, t := range tables {
		out[i] = system.DatabaseTable(t)
	}
	return out
}

// databaseEvents announces migration admission-state changes on the SSE bus.
type databaseEvents struct{ bus *events.Bus }

func (e databaseEvents) Changed(st databasemigration.Status) {
	if e.bus == nil {
		return
	}
	e.bus.Publish(events.Event{Type: "database", Payload: opsapi.DatabaseEvent{
		Phase: st.Phase, Parity: st.Parity, Tables: toDatabaseTables(st.Tables), Error: st.Error,
	}})
}
