package api_test

import (
	"os"
	"testing"
)

// TestMain removes the migrated template (api_test.go) the run may have built. `os.Exit` does
// not run deferred functions, so this is the only place that teardown can happen — a `t.Cleanup`
// would fire after the FIRST test and delete the file the other 461 are copying.
func TestMain(m *testing.M) {
	code := m.Run()
	removeTemplate()
	os.Exit(code)
}
