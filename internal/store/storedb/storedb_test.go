package storedb

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

type sqlStateErr string

func (e sqlStateErr) Error() string    { return "pg: " + string(e) }
func (e sqlStateErr) SQLState() string { return string(e) }

type codeErr int

func (e codeErr) Error() string { return fmt.Sprintf("sqlite: code %d", int(e)) }
func (e codeErr) Code() int     { return int(e) }

func TestIsConstraintViolation(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"postgres unique violation", sqlStateErr("23505"), true},
		{"postgres other state", sqlStateErr("40001"), false},
		{"sqlite constraint", codeErr(19), true},
		{"sqlite extended unique", codeErr(2067), true},
		{"sqlite busy", codeErr(5), false},
		{"wrapped postgres", fmt.Errorf("insert: %w", sqlStateErr("23505")), true},
		{"plain error", errors.New("boom"), false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		if got := IsConstraintViolation(tc.err); got != tc.want {
			t.Errorf("%s: IsConstraintViolation = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestPlaceholders(t *testing.T) {
	const q = `UPDATE t SET a = ?, b = ? WHERE id = ?`
	if got := Passthrough(q); got != q {
		t.Errorf("Passthrough = %q, want unchanged", got)
	}
	if got, want := PgPlaceholders(q), `UPDATE t SET a = $1, b = $2 WHERE id = $3`; got != want {
		t.Errorf("PgPlaceholders = %q, want %q", got, want)
	}
}

func TestEpochRoundTrip(t *testing.T) {
	if Epoch(time.Time{}) != 0 || !FromEpoch(0).IsZero() {
		t.Fatal("zero time must encode to 0 and 0 decode to the zero time")
	}
	at := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	if got := FromEpoch(Epoch(at)); !got.Equal(at) || got.Location() != time.UTC {
		t.Fatalf("round trip = %v, want %v in UTC", got, at)
	}
}

func TestWithPostgresAdvisoryLockRefusesSQLite(t *testing.T) {
	called := false
	err := WithPostgresAdvisoryLock(context.Background(), nil, DialectSQLite, "label", "ns:", "key",
		func(context.Context) error { called = true; return nil })
	if err == nil || called {
		t.Fatalf("err = %v, called = %v; want an unsupported-dialect error and no callback", err, called)
	}
}
