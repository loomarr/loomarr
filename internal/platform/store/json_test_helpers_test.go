package store

import (
	"encoding/json"
	"reflect"
)

// asWritten returns want when got is the same JSON document, else got. Postgres keeps JSON as
// jsonb (#2793), which returns a document with its own spacing and key order, so a round trip
// is compared as documents: a test passes the read-back value through this before comparing.
func asWritten(got, want string) string {
	var g, w any
	if json.Unmarshal([]byte(got), &g) != nil || json.Unmarshal([]byte(want), &w) != nil {
		return got
	}
	if reflect.DeepEqual(g, w) {
		return want
	}
	return got
}
