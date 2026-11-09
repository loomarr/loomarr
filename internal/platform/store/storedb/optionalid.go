package storedb

import (
	"database/sql/driver"
	"fmt"
)

// OptionalID is a nullable foreign key column held as a string: "" (no row named) is stored as
// NULL, which a foreign key allows, and NULL reads back as "". A row's id is never "", so the
// mapping loses nothing (#2793).
//
// ⚠ NULL never equals NULL in SQL. A statement that matches such a column against another, or
// against an argument that may be "", compares with IS NOT DISTINCT FROM, not =.
type OptionalID string

// Value writes "" as NULL.
func (id OptionalID) Value() (driver.Value, error) {
	if id == "" {
		return nil, nil
	}
	return string(id), nil
}

// Scan reads NULL as "".
func (id *OptionalID) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*id = ""
	case string:
		*id = OptionalID(v)
	case []byte:
		*id = OptionalID(v)
	default:
		return fmt.Errorf("optional id: unsupported type %T", src)
	}
	return nil
}
