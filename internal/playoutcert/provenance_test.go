package playoutcert

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/httpfixture"
)

func TestProvenanceFixedVocabularyDoesNotImplySourceLeak(t *testing.T) {
	capsule := workCapsule("mint", probeSecret)
	jsonDocument, reason, ok := parseJSONProvenance([]byte(`{"phases":[{"name":"mint"}]}`), false, newAuditWork(time.Now().Add(time.Second)))
	if !ok {
		t.Fatalf("parse JSON: %q", reason)
	}
	summary := renderReportSummary(Report{Phases: []Phase{{Name: "mint"}}})
	for _, document := range []auditDocument{jsonDocument, summary} {
		if status, reason := auditDocuments(capsule, time.Now().Add(time.Second), document); status != AuditPassed || reason != "" {
			t.Fatalf("fixed phase audit = %q, %q; want passed", status, reason)
		}
	}
}

func TestProvenanceProducerVocabularyDoesNotCollideWithPreparedProbe(t *testing.T) {
	endpoint, capsule := auditEndpoint(t, httpfixture.NewScriptedTransport(step(204, "")))
	signed, err := url.Parse("https://fixture.invalid/hls/master.m3u8?sig=token")
	if err != nil {
		t.Fatal(err)
	}
	if _, hit, class := endpoint.prepared(context.Background(), signed); hit || class != "prepared_miss" {
		t.Fatalf("prepared = hit:%t class:%q", hit, class)
	}

	jsonDocument, reason, ok := parseJSONProvenance([]byte(`{"phases":[{"httpClasses":{"prepared_miss":1},"programmeBoundaries":[{"lane":"prepared"}]}]}`), false, newAuditWork(time.Now().Add(time.Second)))
	if !ok {
		t.Fatalf("parse JSON: %q", reason)
	}
	summary := renderReportSummary(Report{Phases: []Phase{{Name: "programme_boundary", Failures: 1, HTTPClasses: map[string]int{"prepared_miss": 1}}}})
	for _, document := range []auditDocument{jsonDocument, summary} {
		if status, reason := auditDocuments(capsule, time.Now().Add(time.Second), document); status != AuditPassed || reason != "" {
			t.Fatalf("producer vocabulary audit = %q, %q; want passed", status, reason)
		}
	}

	for _, raw := range []string{
		`{"phases":[{"httpClasses":{"prepared":1}}]}`,
		`{"target":{"version":"prepared"}}`,
	} {
		document, reason, ok := parseJSONProvenance([]byte(raw), false, newAuditWork(time.Now().Add(time.Second)))
		if !ok {
			t.Fatalf("parse JSON: %q", reason)
		}
		if status, reason := auditDocuments(capsule, time.Now().Add(time.Second), document); status != AuditUnavailable || reason != AuditReasonDynamicCollision {
			t.Fatalf("arbitrary value audit %q = %q, %q; want unavailable dynamic collision", raw, status, reason)
		}
	}
}

func TestProvenanceIndependentCollisionIsUnavailableEvenForSecretProbe(t *testing.T) {
	for _, raw := range []string{
		`{"target":{"version":"bearer"}}`,
		`{"target":{"capacity":7}}`,
	} {
		probe := "bearer"
		if raw == `{"target":{"capacity":7}}` {
			probe = "7"
		}
		document, reason, ok := parseJSONProvenance([]byte(raw), false, newAuditWork(time.Now().Add(time.Second)))
		if !ok {
			t.Fatalf("parse JSON: %q", reason)
		}
		status, gotReason := auditDocuments(workCapsule(probe, probeSecret), time.Now().Add(time.Second), document)
		if status != AuditUnavailable || gotReason != AuditReasonDynamicCollision {
			t.Fatalf("audit %q = %q, %q; want unavailable dynamic collision", raw, status, gotReason)
		}
	}
}

func TestProvenanceRejectsUnclosedVocabularyAndMapKeys(t *testing.T) {
	for _, raw := range []string{
		`{"phases":[{"name":"mint-secret"}]}`,
		`{"faultProfiles":[{"outcome":"secret"}]}`,
		`{"phases":[{"httpClasses":{"secret":1}}]}`,
	} {
		document, reason, ok := parseJSONProvenance([]byte(raw), false, newAuditWork(time.Now().Add(time.Second)))
		if !ok {
			t.Fatalf("parse JSON: %q", reason)
		}
		status, gotReason := auditDocuments(workCapsule("secret", probeCollision), time.Now().Add(time.Second), document)
		if status != AuditUnavailable || gotReason != AuditReasonDynamicCollision {
			t.Fatalf("audit %q = %q, %q; want unavailable dynamic collision", raw, status, gotReason)
		}
	}
}

func TestProvenanceEscapedJSONAndSummaryAgree(t *testing.T) {
	probe := "a bc"
	jsonDocument, reason, ok := parseJSONProvenance([]byte(`{"target":{"version":"a\u0020bc"}}`), false, newAuditWork(time.Now().Add(time.Second)))
	if !ok {
		t.Fatalf("parse JSON: %q", reason)
	}
	summary := renderReportSummary(Report{Target: Target{Version: probe}})
	for _, document := range []auditDocument{jsonDocument, summary} {
		status, gotReason := auditDocuments(workCapsule(probe, probeCollision), time.Now().Add(time.Second), document)
		if status != AuditUnavailable || gotReason != AuditReasonDynamicCollision {
			t.Fatalf("audit = %q, %q; want unavailable dynamic collision", status, gotReason)
		}
	}
}

func TestProvenanceSourceLeakAndMissingProvenanceFailClosed(t *testing.T) {
	for _, document := range []auditDocument{
		{raw: []byte("secret"), spans: []provenanceSpan{{start: 0, end: 6, kind: provenanceSensitive}}},
		{raw: []byte("secret"), spans: []provenanceSpan{{start: 0, end: 6}}},
	} {
		status, reason := auditDocuments(workCapsule("secret", probeCollision), time.Now().Add(time.Second), document)
		if document.spans[0].kind == provenanceSensitive {
			if status != AuditFailed || reason != AuditReasonSensitiveValue {
				t.Fatalf("sensitive audit = %q, %q; want failed sensitive value", status, reason)
			}
			continue
		}
		if status != AuditUnavailable || reason != AuditReasonProvenanceMissing {
			t.Fatalf("unknown audit = %q, %q; want unavailable provenance missing", status, reason)
		}
	}
}

func TestProvenanceFallbackFixedValidationIsSafe(t *testing.T) {
	document, reason, ok := parseJSONProvenance([]byte(`{"schemaVersion":3,"certified":false,"auditStatus":"unavailable","auditReason":"provenance_missing"}`), true, newAuditWork(time.Now().Add(time.Second)))
	if !ok {
		t.Fatalf("parse fallback JSON: %q", reason)
	}
	if status, reason := auditDocuments(workCapsule("secret", probeSecret), time.Now().Add(time.Second), document); status != AuditPassed || reason != "" {
		t.Fatalf("fallback audit = %q, %q; want passed", status, reason)
	}
	unknown, reason, ok := parseJSONProvenance([]byte(`{"anything":"secret"}`), true, newAuditWork(time.Now().Add(time.Second)))
	if !ok {
		t.Fatalf("parse unknown fallback JSON: %q", reason)
	}
	if status, reason := auditDocuments(workCapsule("secret", probeSecret), time.Now().Add(time.Second), unknown); status != AuditUnavailable || reason != AuditReasonDynamicCollision {
		t.Fatalf("unknown fallback audit = %q, %q; want unavailable dynamic collision", status, reason)
	}
}
