package playoutbench

import (
	"os"
	"testing"
)

// The production table of known failures would turn every hand-built "green" report in these tests
// into an unexpected pass; the tests that exercise the mechanism set their own table.
var productionExpectedFailures map[string]string

func TestMain(m *testing.M) {
	saved := expectedFailures
	productionExpectedFailures = saved
	expectedFailures = nil
	code := m.Run()
	expectedFailures = saved
	os.Exit(code)
}
