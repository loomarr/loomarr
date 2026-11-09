package storetest

import "testing"

// TestStatementVerbSeesPastComments: a leading comment cannot hide a statement from the plan
// check; only the explicit ReadsWholeMarker excuses one.
func TestStatementVerbSeesPastComments(t *testing.T) {
	for query, want := range map[string]string{
		"SELECT 1":                                   "SELECT",
		"  with x AS (SELECT 1) SELECT * FROM x":     "SELECT",
		"/* lookup */ SELECT id FROM t WHERE k = ?":  "SELECT",
		"-- note\nDELETE FROM t WHERE k = ?":         "DELETE",
		"/* a */ -- b\n UPDATE t SET k = 1":          "UPDATE",
		ReadsWholeMarker + " SELECT COUNT(*) FROM t": "",
		"PRAGMA optimize":                            "",
		"/* never closed SELECT 1":                   "",
	} {
		if got := statementVerb(query); got != want {
			t.Errorf("statementVerb(%q) = %q, want %q", query, got, want)
		}
	}
}
