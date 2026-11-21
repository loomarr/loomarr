package tools

import (
	"reflect"
	"testing"
)

func TestParseLoudness(t *testing.T) {
	out := "frame:0    pts:0       pts_time:0\n" +
		"lavfi.astats.Overall.RMS_level=-23.456\n" +
		"frame:1    pts:9600    pts_time:0.2\n" +
		"lavfi.astats.Overall.RMS_level=-inf\n" +
		"lavfi.astats.Overall.RMS_level=nan\n" +
		"lavfi.astats.Overall.RMS_level=-120.0\n"
	want := []float64{-23.456, SilentDb, SilentDb, SilentDb}
	if got := parseLoudness(out); !reflect.DeepEqual(got, want) {
		t.Errorf("levels = %v, want %v", got, want)
	}
}
