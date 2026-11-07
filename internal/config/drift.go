package config

import "reflect"

// ChangedKeys names the boot keys whose value in desired differs from c, in field order. Every
// env-tagged field counts: a generation reads all of them when it starts, so each one waits on a
// restart. DataDir has no env name and is recorded once, so it never drifts. A nil side has no
// baseline and reports nothing.
func (c *Config) ChangedKeys(desired *Config) []string {
	if c == nil || desired == nil {
		return nil
	}
	var changed []string
	running, want := reflect.ValueOf(c).Elem(), reflect.ValueOf(desired).Elem()
	for _, f := range reflect.VisibleFields(running.Type()) {
		key := f.Tag.Get("env")
		if key == "" || key == "-" {
			continue
		}
		if !running.FieldByIndex(f.Index).Equal(want.FieldByIndex(f.Index)) {
			changed = append(changed, key)
		}
	}
	return changed
}
