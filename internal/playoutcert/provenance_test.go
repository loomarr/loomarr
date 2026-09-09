package playoutcert

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/httpfixture"
	"github.com/loomarr/loomarr/internal/testkit/playoutcertfixture"
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

func TestProvenanceProgrammeSignalVocabularyIsScoped(t *testing.T) {
	for _, word := range []string{"asset_clock_mismatch", "programme_observation_timeout", "unexpected_media_eof", "invalid_media_clock", "media_outside_truth", "invalid_video_signal", "invalid_audio_signal", "video_time_regressed", "audio_time_regressed", "programme_video_mismatch", "programme_audio_mismatch"} {
		for _, raw := range []string{`{"phases":[{"httpClasses":{"` + word + `":1}}]}`, `{"phases":[{"programmeBoundaries":[{"outcome":"` + word + `","decodedAudioSamplesDelta":2048}]}]}`} {
			doc, reason, ok := parseJSONProvenance([]byte(raw), false, newAuditWork(time.Now().Add(time.Second)))
			if !ok {
				t.Fatalf("parse: %s", reason)
			}
			if status, reason := auditDocuments(workCapsule(word, probeCollision), time.Now().Add(time.Second), doc); status != AuditPassed {
				t.Fatalf("fixed signal vocabulary %s: %s %s", word, status, reason)
			}
		}
		doc, reason, ok := parseJSONProvenance([]byte(`{"target":{"version":"`+word+`"}}`), false, newAuditWork(time.Now().Add(time.Second)))
		if !ok {
			t.Fatalf("parse: %s", reason)
		}
		if status, reason := auditDocuments(workCapsule(word, probeCollision), time.Now().Add(time.Second), doc); status != AuditUnavailable || reason != AuditReasonDynamicCollision {
			t.Fatalf("dynamic signal-word collision %s: %s %s", word, status, reason)
		}
	}
}

func TestProgrammePrivateInputsParticipateInPublicationAudit(t *testing.T) {
	source := &playoutcertfixture.ProgrammeEvidence[ProgrammeEvidence]{Private: []string{"/private-corpus/sensitive-title.mov"}}
	capsule := newAuditCapsule(Config{BaseURL: "http://127.0.0.1", AdminBearer: "private-admin", DeviceToken: "private-device", ProgrammeEvidence: source})
	document, reason, ok := parseJSONProvenance([]byte(`{"target":{"version":"/private-corpus/sensitive-title.mov"}}`), false, newAuditWork(time.Now().Add(time.Second)))
	if !ok {
		t.Fatal(reason)
	}
	if status, reason := auditDocuments(capsule, time.Now().Add(time.Second), document); status != AuditUnavailable || reason != AuditReasonDynamicCollision {
		t.Fatalf("private cohort input escaped audit: %s %s", status, reason)
	}
}

func TestCohortDigestRemainsDynamicPublicationData(t *testing.T) {
	digest := strings.Repeat("a", 64)
	document, reason, ok := parseJSONProvenance([]byte(`{"target":{"cohortManifestSha256":"`+digest+`"}}`), false, newAuditWork(time.Now().Add(time.Second)))
	if !ok {
		t.Fatal(reason)
	}
	if status, reason := auditDocuments(workCapsule(digest, probeCollision), time.Now().Add(time.Second), document); status != AuditUnavailable || reason != AuditReasonDynamicCollision {
		t.Fatalf("digest collision bypassed audit: %s %s", status, reason)
	}
	if status, reason := auditDocuments(workCapsule("unrelated private value", probeCollision), time.Now().Add(time.Second), document); status != AuditPassed {
		t.Fatalf("independent digest failed audit: %s %s", status, reason)
	}
}
