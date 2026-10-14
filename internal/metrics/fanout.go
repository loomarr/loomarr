package metrics

import (
	"context"
	"sync/atomic"
)

type fanoutKeyType struct{}

var fanoutKey fanoutKeyType

func withFanoutCounter(ctx context.Context) (context.Context, *atomic.Int64) {
	count := &atomic.Int64{}
	return context.WithValue(ctx, fanoutKey, count), count
}

func countOutbound(ctx context.Context) {
	if count, ok := ctx.Value(fanoutKey).(*atomic.Int64); ok {
		count.Add(1)
	}
}
