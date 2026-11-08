package storedb

import (
	"database/sql/driver"
	"fmt"
)

// OptionalJSON is a JSON document that may be absent: "" in Go, NULL in the database. A JSON column
// is jsonb in Postgres and CHECKed with json_valid in SQLite (#2793), and neither accepts "" as a
// document, so absence is NULL. Pass OptionalJSON(s) as a query argument and scan into
// (*OptionalJSON)(&s); the Go field stays a plain string.
type OptionalJSON string

// Value writes NULL for an absent document.
func (j OptionalJSON) Value() (driver.Value, error) {
	if j == "" {
		return nil, nil
	}
	return string(j), nil
}

// Scan reads NULL as an absent document. Postgres returns jsonb as text or bytes.
func (j *OptionalJSON) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*j = ""
	case string:
		*j = OptionalJSON(v)
	case []byte:
		*j = OptionalJSON(v)
	default:
		return fmt.Errorf("storedb.OptionalJSON: cannot scan %T", src)
	}
	return nil
}
