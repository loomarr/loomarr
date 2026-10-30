package storedb

import "errors"

// ErrNotFound reports that a requested durable identity does not exist. Besides Get* reads,
// commands whose integrity depends on an existing owner (such as channel-scoped feedback) return
// it before committing any dependent row.
//
// internal/store.ErrNotFound is this same value, so errors.Is matches either name.
var ErrNotFound = errors.New("store: not found")

// IsConstraintViolation recognizes the portable driver contracts without coupling the
// store to either concrete error package. Postgres exposes SQLSTATE 23505; modernc SQLite
// exposes extended result codes whose low byte is SQLITE_CONSTRAINT (19).
func IsConstraintViolation(err error) bool {
	var state interface{ SQLState() string }
	if errors.As(err, &state) && state.SQLState() == "23505" {
		return true
	}
	var coded interface{ Code() int }
	return errors.As(err, &coded) && coded.Code()&0xff == 19
}
