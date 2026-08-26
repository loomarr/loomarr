package testkit

import (
	"context"
	"fmt"

	"github.com/loomarr/loomarr/internal/fillerbakeoff"
)

// FillerBakeoffExtractor is the shared deterministic adapter for bakeoff tests.
type FillerBakeoffExtractor struct {
	Results  []fillerbakeoff.Extraction
	Errors   []error
	Requests []fillerbakeoff.Request
}

func (f *FillerBakeoffExtractor) Extract(_ context.Context, request fillerbakeoff.Request) (fillerbakeoff.Extraction, error) {
	f.Requests = append(f.Requests, request)
	index := len(f.Requests) - 1
	if index < len(f.Errors) && f.Errors[index] != nil {
		return fillerbakeoff.Extraction{}, f.Errors[index]
	}
	if index >= len(f.Results) {
		return fillerbakeoff.Extraction{}, fmt.Errorf("no filler bakeoff result %d", index)
	}
	return f.Results[index], nil
}
