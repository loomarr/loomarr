package playoutcert

import (
	"fmt"
	"sort"
	"strconv"
)

func HumanSummary(report Report) string {
	return string(minimalSummary(AuditMissing, AuditReasonNotFinalized).raw)
}

func renderReportSummary(report Report) auditDocument {
	var out provenanceBuilder
	verdict := "FAIL"
	if report.Certified {
		verdict = "PASS"
	}
	out.fixed("Playout load certification: ")
	out.fixed(verdict)
	out.fixed("\nTarget: ")
	out.dynamic(report.Target.Version)
	out.fixed(" (")
	out.dynamic(shortRevision(report.Target.Revision))
	out.fixed("), ")
	out.dynamic(strconv.Itoa(report.Target.ConfiguredChannels))
	out.fixed(" configured Channels, capacity ")
	out.dynamic(strconv.Itoa(report.Target.Capacity))
	out.fixed("\n")
	for _, phase := range report.Phases {
		out.append(fmt.Sprintf("%-12s", phase.Name), provenanceForSummary("phase", phase.Name))
		out.fixed(" attempts=")
		out.dynamic(strconv.Itoa(phase.Attempts))
		out.fixed(" failures=")
		out.dynamic(strconv.Itoa(phase.Failures))
		out.fixed(" p50=")
		out.dynamic(fmt.Sprintf("%.1f", phase.P50MS))
		out.fixed("ms p95=")
		out.dynamic(fmt.Sprintf("%.1f", phase.P95MS))
		out.fixed("ms p99=")
		out.dynamic(fmt.Sprintf("%.1f", phase.P99MS))
		out.fixed("ms")
		if phase.PreparedHits > 0 {
			out.fixed(" prepared=")
			out.dynamic(strconv.Itoa(phase.PreparedHits))
		}
		if phase.FirstByte.Attempts > 0 {
			out.fixed(" first-byte-p95=")
			out.dynamic(fmt.Sprintf("%.1f", phase.FirstByte.P95MS))
			out.fixed("ms")
		}
		if len(phase.HTTPClasses) > 1 || phase.Failures > 0 {
			classes := make([]string, 0, len(phase.HTTPClasses))
			for class := range phase.HTTPClasses {
				classes = append(classes, class)
			}
			sort.Strings(classes)
			out.fixed(" classes=")
			for index, class := range classes {
				if index != 0 {
					out.fixed(",")
				}
				out.append(class, provenanceForSummary("httpClass", class))
				out.fixed(":")
				out.dynamic(strconv.Itoa(phase.HTTPClasses[class]))
			}
		}
		out.fixed("\n")
	}
	if len(report.FaultProfiles) > 0 {
		out.fixed("Fault profiles:\n")
		for _, qualification := range report.FaultProfiles {
			out.append(string(qualification.Profile), provenanceForSummary("faultProfile", string(qualification.Profile)))
			out.fixed(" status=")
			out.append(qualification.Status, provenanceForSummary("faultStatus", qualification.Status))
			out.fixed(" outcome=")
			out.append(qualification.Outcome, provenanceForSummary("faultOutcome", qualification.Outcome))
			out.fixed("\n")
		}
	}
	if len(report.Failures) > 0 {
		out.fixed("Failures: ")
		for index, failure := range report.Failures {
			if index > 0 {
				out.fixed(", ")
			}
			out.append(failure, provenanceForSummary("failure", failure))
		}
		out.fixed("\n")
	}
	return out.document()
}

func provenanceForSummary(kind, value string) provenance {
	if fixedSummaryValue(kind, value) {
		return provenanceFixed
	}
	return provenanceDynamic
}

func minimalSummary(status AuditStatus, reason AuditReason) auditDocument {
	var out provenanceBuilder
	out.fixed("Playout load certification: FAIL\nAudit: ")
	out.fixed(string(status))
	out.fixed(" (")
	out.fixed(string(reason))
	out.fixed(")\n")
	return out.document()
}

func shortRevision(revision string) string {
	if len(revision) > 12 {
		return revision[:12]
	}
	return revision
}
