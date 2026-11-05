package config

import (
	"maps"
	"reflect"
	"slices"
	"testing"
)

// Every boot key a generation reads is restart-scoped, so ChangedKeys names whichever one the
// desired config moved. The table must cover every env-tagged field: a boot key added without a
// row here fails, rather than silently never asking for a restart.
func TestChangedKeysNamesEveryBootKey(t *testing.T) {
	changed := map[string]string{
		"LISTEN_ADDR":                        ":9090",
		"LOG_LEVEL":                          "debug",
		"LOOMARR_AGENT_DISABLE_LIVETV_TUNER": "1",
		"DATABASE_URL":                       "postgres://u:p@h:5432/loomarr",
		"AUTO_MIGRATE":                       "false",
		"LOOMARR_DEV_LOGIN":                  "true",
		"LOOMARR_PPROF":                      "true",
		"LOOMARR_METRICS_TOKEN":              "scrape-token",
	}
	if got, want := bootKeys(), slices.Sorted(maps.Keys(changed)); !slices.Equal(slices.Sorted(slices.Values(got)), want) {
		t.Fatalf("boot keys = %v, table covers %v; add a row for each new boot key", got, want)
	}

	running, err := LoadEnv(map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if got := running.ChangedKeys(running); len(got) != 0 {
		t.Errorf("ChangedKeys(self) = %v, want none", got)
	}
	for key, value := range changed {
		desired, err := LoadEnv(map[string]string{key: value})
		if err != nil {
			t.Fatalf("%s=%s: %v", key, value, err)
		}
		if got := running.ChangedKeys(desired); !slices.Equal(got, []string{key}) {
			t.Errorf("%s changed: ChangedKeys = %v, want [%s]", key, got, key)
		}
	}
}

// A nil side has no baseline to compare, so nothing is reported: a false "restart required"
// points the operator at an action that cannot help.
func TestChangedKeysWithoutABaseline(t *testing.T) {
	c, err := LoadEnv(map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	var none *Config
	if got := none.ChangedKeys(c); got != nil {
		t.Errorf("nil.ChangedKeys = %v, want nil", got)
	}
	if got := c.ChangedKeys(nil); got != nil {
		t.Errorf("ChangedKeys(nil) = %v, want nil", got)
	}
}

// bootKeys lists the env names of Config's boot keys, in field order.
func bootKeys() []string {
	var keys []string
	for f := range reflect.TypeFor[Config]().Fields() {
		if key := f.Tag.Get("env"); key != "" && key != "-" {
			keys = append(keys, key)
		}
	}
	return keys
}
