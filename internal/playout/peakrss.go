package playout

import (
	"os"
	"reflect"
	"runtime"
)

// peakRSSBytes is a finished child's peak resident set, or 0 when the platform does not report it.
// It reads ru_maxrss through reflection so one file compiles everywhere without platform build tags:
// Linux reports KiB, Darwin reports bytes, and Windows has no such field.
func peakRSSBytes(state *os.ProcessState) int64 {
	if state == nil {
		return 0
	}
	return maxRSSBytes(state.SysUsage(), runtime.GOOS)
}

func maxRSSBytes(usage any, goos string) int64 {
	v := reflect.ValueOf(usage)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return 0
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return 0
	}
	field := v.FieldByName("Maxrss")
	if !field.IsValid() || !field.CanInt() || field.Int() <= 0 {
		return 0
	}
	switch goos {
	case "linux":
		return field.Int() * 1024
	case "darwin":
		return field.Int()
	default:
		return 0 // a platform whose unit is not known here is reported as unmeasured
	}
}
