package playoutcert

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func workCapsule(value string, disposition probeDisposition) *auditCapsule {
	return &auditCapsule{available: true, probes: []auditProbe{{value: value, disposition: disposition}}}
}

func TestAuditWorkExpiredWithoutProbesFailsClosed(t *testing.T) {
	capsule := &auditCapsule{available: true}
	status, reason := auditDocuments(capsule, time.Now().Add(-time.Millisecond), auditDocument{})
	if status != AuditUnavailable || reason != AuditReasonDeadline {
		t.Fatalf("audit = %q, %q; want unavailable deadline", status, reason)
	}
}

func TestAuditWorkFrequentFixedMatchesUsesMovingSpans(t *testing.T) {
	const count = 256
	raw := []byte(strings.Repeat("x", count))
	spans := make([]provenanceSpan, count)
	for i := range spans {
		spans[i] = provenanceSpan{start: i, end: i + 1, fixed: true}
	}
	matched, available := unsafeRawMatch(auditDocument{raw: raw, spans: spans}, "x", newAuditWorkBudget(time.Now().Add(time.Second), count*2+1))
	if matched != provenanceFixed || !available {
		t.Fatalf("fixed matches = %v, available = %t; want fixed, true", matched, available)
	}
}

func TestAuditWorkExhaustionFailsClosed(t *testing.T) {
	_, available := unsafeRawMatch(auditDocument{raw: []byte(strings.Repeat("x", 16)), spans: []provenanceSpan{{start: 0, end: 16, fixed: true}}}, "x", newAuditWorkBudget(time.Now().Add(time.Second), 1))
	if available {
		t.Fatal("work exhaustion was accepted")
	}
}

func TestAuditWorkCrossFixedDynamicSpanAndDecodedCollision(t *testing.T) {
	document := auditDocument{
		raw:   []byte("abcabc"),
		spans: []provenanceSpan{{start: 0, end: 3, fixed: true}, {start: 3, end: 6, kind: provenanceDynamic}},
	}
	if matched, available := unsafeRawMatch(document, "abc", newAuditWorkBudget(time.Now().Add(time.Second), 16)); matched != provenanceDynamic || !available {
		t.Fatalf("raw collision = %v, %t; want dynamic, true", matched, available)
	}
	escaped, reason, ok := parseJSONProvenance([]byte(`{"dynamic":"a\u0062c"}`), false, newAuditWorkBudget(time.Now().Add(time.Second), 64))
	if !ok {
		t.Fatalf("escaped JSON parse failed: %q", reason)
	}
	if matched, available := unsafeDecodedMatch(escaped, "abc", newAuditWorkBudget(time.Now().Add(time.Second), 16)); matched != provenanceDynamic || !available {
		t.Fatalf("decoded collision = %v, %t; want dynamic, true", matched, available)
	}
}

func TestAuditWorkParserWhitespaceAndLongStringExhaustion(t *testing.T) {
	if _, reason, ok := parseJSONProvenance([]byte("  \n\t{\"value\":\""+strings.Repeat("x", 128)+"\"}"), false, newAuditWorkBudget(time.Now().Add(time.Second), 8)); ok || reason != AuditReasonDeadline {
		t.Fatalf("parser = ok %t, reason %q; want false deadline", ok, reason)
	}
}

func TestAuditWorkMaterialAndOutputBounds(t *testing.T) {
	capsule := workCapsule("safe", probeCollision)
	capsule.registerSource(strings.Repeat("x", maxAuditMatcherBytes+1), probeSecret)
	if _, reason, ok := capsule.snapshot(); ok || reason != AuditReasonMatcherLimit {
		t.Fatalf("oversized source = ok %t, reason %q; want false matcher limit", ok, reason)
	}
	remaining := int64(maxPublicationOutputBytes)
	if boundedJSONValue(reflect.ValueOf(strings.Repeat("x", maxPublicationOutputBytes)), &remaining, time.Now().Add(time.Second), 0, new(int)) {
		t.Fatal("oversized output input was accepted")
	}
}

func TestAuditWorkHeldCapsuleHonorsDeadline(t *testing.T) {
	capsule := workCapsule("probe", probeCollision)
	capsule.mu.Lock()
	defer capsule.mu.Unlock()
	started := time.Now()
	status, reason := auditDocuments(capsule, time.Now().Add(5*time.Millisecond), auditDocument{})
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("held capsule took %s", elapsed)
	}
	if status != AuditUnavailable || reason != AuditReasonDeadline {
		t.Fatalf("held capsule audit = %q, %q; want unavailable deadline", status, reason)
	}
}
