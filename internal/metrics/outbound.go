package metrics

// OutboundRetryReason is a closed reason for an actual additional HTTP attempt.
type OutboundRetryReason uint8

const (
	OutboundRetryTransport OutboundRetryReason = iota
	OutboundRetryStatus408
	OutboundRetryStatus429
	OutboundRetryStatus500
	OutboundRetryStatus502
	OutboundRetryStatus503
	OutboundRetryStatus504
	// OutboundRetryStatusOther is any other transient status (425, 507, 511, a CDN's 52x).
	OutboundRetryStatusOther
)

var outboundRetryReasonLabels = [...]string{
	"transport", "408", "429", "500", "502", "503", "504", "other_status",
}
