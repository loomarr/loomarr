package store

import "embed"

// Embedded migration FS (§5): separate per-dialect dirs so dialect DDL never
// leaks across backends. goose selects the dir matching the active driver.
//
//go:embed migrations/sqlite/*.sql migrations/postgres/*.sql
var migrationFS embed.FS
