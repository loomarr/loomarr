package playoutcert

import (
	"fmt"
	"strings"
)

func HumanSummary(report Report) string {
	var out strings.Builder
	verdict := "FAIL"
	if report.Certified {
		verdict = "PASS"
	}
	_, _ = fmt.Fprintf(&out, "Playout load certification: %s\n", verdict)
	_, _ = fmt.Fprintf(&out, "Target: %s (%s), %d configured Channels, capacity %d\n",
		report.Target.Version, shortRevision(report.Target.Revision), report.Target.ConfiguredChannels, report.Target.Capacity)
	for _, phase := range report.Phases {
		_, _ = fmt.Fprintf(&out, "%-12s attempts=%d failures=%d p50=%.1fms p95=%.1fms p99=%.1fms",
			phase.Name, phase.Attempts, phase.Failures, phase.P50MS, phase.P95MS, phase.P99MS)
		if phase.PreparedHits > 0 {
			_, _ = fmt.Fprintf(&out, " prepared=%d", phase.PreparedHits)
		}
		out.WriteByte('\n')
	}
	if len(report.Failures) > 0 {
		_, _ = fmt.Fprintf(&out, "Failures: %s\n", strings.Join(report.Failures, ", "))
	}
	return out.String()
}

func shortRevision(revision string) string {
	if len(revision) > 12 {
		return revision[:12]
	}
	return revision
}
