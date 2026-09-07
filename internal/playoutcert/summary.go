package playoutcert

import (
	"fmt"
	"sort"
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
		if phase.FirstByte.Attempts > 0 {
			_, _ = fmt.Fprintf(&out, " first-byte-p95=%.1fms", phase.FirstByte.P95MS)
		}
		if len(phase.HTTPClasses) > 1 || phase.Failures > 0 {
			classes := make([]string, 0, len(phase.HTTPClasses))
			for class, count := range phase.HTTPClasses {
				classes = append(classes, fmt.Sprintf("%s:%d", class, count))
			}
			sort.Strings(classes)
			_, _ = fmt.Fprintf(&out, " classes=%s", strings.Join(classes, ","))
		}
		out.WriteByte('\n')
	}
	if len(report.FaultProfiles) > 0 {
		_, _ = fmt.Fprintln(&out, "Fault profiles:")
		for _, qualification := range report.FaultProfiles {
			_, _ = fmt.Fprintf(&out, "%s status=%s outcome=%s\n", qualification.Profile, qualification.Status, qualification.Outcome)
		}
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
