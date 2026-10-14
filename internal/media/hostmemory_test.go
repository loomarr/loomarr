package media

import (
	"strings"
	"testing"
)

func TestParseMemAvailable(t *testing.T) {
	const meminfo = "MemTotal:       32768000 kB\nMemFree:         1600000 kB\nMemAvailable:    6082560 kB\nBuffers:           10000 kB\n"
	got, ok := parseMemAvailable(strings.NewReader(meminfo))
	if !ok || got != 6082560*1024 {
		t.Fatalf("parseMemAvailable = %d, %v; want %d, true", got, ok, int64(6082560*1024))
	}
	for name, content := range map[string]string{
		"absent":    "MemTotal: 1 kB\n",
		"negative":  "MemAvailable: -5 kB\n",
		"garbage":   "MemAvailable: lots kB\n",
		"wrongUnit": "MemAvailable: 5 MB\n",
		"empty":     "",
	} {
		if _, ok := parseMemAvailable(strings.NewReader(content)); ok {
			t.Errorf("%s: parseMemAvailable accepted %q", name, content)
		}
	}
}
