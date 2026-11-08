package storetest

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"runtime"
	"strings"
	"sync"

	_ "modernc.org/sqlite" // the driver a Recorder wraps
)

// Recorder is a database/sql driver over SQLite that notes every distinct statement the store
// sends, with the arguments it first ran with, so CheckQueryPlans can ask SQLite how it plans
// each one. Statements a test issues itself (reading the database back to assert) are not noted.
type Recorder struct {
	mu   sync.Mutex
	args map[string][]any
}

// NewRecorder registers a Recorder as the database/sql driver driverName. Call it once per test
// binary, from a package-level var: database/sql panics on a second registration of a name.
func NewRecorder(driverName string) *Recorder {
	inner, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		panic(err)
	}
	r := &Recorder{args: map[string][]any{}}
	sql.Register(driverName, recordingDriver{inner: inner.Driver(), rec: r})
	_ = inner.Close()
	return r
}

// Statements copies what has been recorded so far.
func (r *Recorder) Statements() map[string][]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string][]any, len(r.args))
	for q, a := range r.args {
		out[q] = a
	}
	return out
}

func (r *Recorder) note(query string, named []driver.NamedValue) {
	if issuedByTest() {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, seen := r.args[query]; seen {
		return
	}
	args := make([]any, len(named))
	for i, n := range named {
		args[i] = n.Value
	}
	r.args[query] = args
}

// issuedByTest reports whether the statement came straight from test code rather than from the
// store. The first caller outside database/sql and this file decides: a _test.go frame there is
// the test itself.
func issuedByTest() bool {
	pcs := make([]uintptr, 32)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(3, pcs)])
	for {
		f, more := frames.Next()
		if !strings.HasPrefix(f.Function, "database/sql.") && !strings.HasSuffix(f.File, "storetest/planrecorder.go") {
			return strings.HasSuffix(f.File, "_test.go")
		}
		if !more {
			return false
		}
	}
}

type recordingDriver struct {
	inner driver.Driver
	rec   *Recorder
}

func (d recordingDriver) Open(name string) (driver.Conn, error) {
	c, err := d.inner.Open(name)
	if err != nil {
		return nil, err
	}
	return recordingConn{Conn: c, rec: d.rec}, nil
}

// recordingConn forwards to the SQLite connection, noting each statement first. It implements
// the optional interfaces database/sql looks for, so the store runs exactly as it does in
// production.
type recordingConn struct {
	driver.Conn
	rec *Recorder
}

var errNoBeginTx = errors.New("storetest: the wrapped driver cannot begin a transaction with options")

func (c recordingConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.rec.note(query, args)
	q, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	return q.QueryContext(ctx, query, args)
}

func (c recordingConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.rec.note(query, args)
	e, ok := c.Conn.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	return e.ExecContext(ctx, query, args)
}

func (c recordingConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	c.rec.note(query, nil)
	if p, ok := c.Conn.(driver.ConnPrepareContext); ok {
		return p.PrepareContext(ctx, query)
	}
	return c.Prepare(query)
}

func (c recordingConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	b, ok := c.Conn.(driver.ConnBeginTx)
	if !ok {
		return nil, errNoBeginTx
	}
	return b.BeginTx(ctx, opts)
}

func (c recordingConn) Ping(ctx context.Context) error {
	if p, ok := c.Conn.(driver.Pinger); ok {
		return p.Ping(ctx)
	}
	return nil
}

func (c recordingConn) ResetSession(ctx context.Context) error {
	if r, ok := c.Conn.(driver.SessionResetter); ok {
		return r.ResetSession(ctx)
	}
	return nil
}

func (c recordingConn) CheckNamedValue(v *driver.NamedValue) error {
	if n, ok := c.Conn.(driver.NamedValueChecker); ok {
		return n.CheckNamedValue(v)
	}
	return driver.ErrSkip
}
