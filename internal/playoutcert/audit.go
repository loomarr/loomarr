package playoutcert

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	maxAuditProbes       = 8192
	maxAuditMatcherBytes = 8 << 20
)

type probeDisposition uint8

const (
	probeCollision probeDisposition = iota
	probeSecret
)

type auditProbe struct {
	value       string
	disposition probeDisposition
}

// auditCapsule is the only owner of values which must never enter a
// publication. It is shared by concurrent request workers, so registration is
// both bounded and synchronized.
type auditCapsule struct {
	mu             sync.Mutex
	probes         []auditProbe
	probeIndex     map[string]int
	matcherBytes   int
	available      bool
	unavailableFor AuditReason
	requiredFaults []FaultProfile
}

func newAuditCapsule(config Config) *auditCapsule {
	capsule := &auditCapsule{
		probeIndex:     make(map[string]int),
		available:      true,
		requiredFaults: append([]FaultProfile(nil), config.FaultProfiles...),
	}
	capsule.registerSource(config.AdminBearer, probeSecret)
	capsule.registerSource("Bearer "+config.AdminBearer, probeSecret)
	capsule.registerSource(config.DeviceToken, probeSecret)
	capsule.registerSource(config.BaseURL, probeCollision)
	if origin, err := url.Parse(config.BaseURL); err == nil {
		capsule.registerURL(origin, probeCollision)
	} else {
		capsule.markUnavailable(AuditReasonEncodingUnsupported)
	}
	for _, channel := range config.Channels {
		capsule.registerSource(channel.ID, probeCollision)
	}
	if config.ProgrammeEvidence != nil {
		for _, input := range config.ProgrammeEvidence.PrivateInputs() {
			capsule.registerSource(input, probeCollision)
		}
	}
	return capsule
}

type requestProvenance uint8

const (
	requestProvenancePrivate requestProvenance = iota
	requestProvenanceDiagnosticsFixedControls
)

func (a *auditCapsule) registerRequestURL(value *url.URL, provenance requestProvenance) bool {
	if value == nil {
		a.markUnavailable(AuditReasonProvenanceMissing)
		return false
	}
	a.registerURLWithProvenance(value, probeCollision, provenance)
	return a.isAvailable()
}

func (a *auditCapsule) registerSignedURL(value *url.URL) bool {
	if value == nil {
		a.markUnavailable(AuditReasonProvenanceMissing)
		return false
	}
	a.registerURL(value, probeCollision)
	return a.isAvailable()
}

func (a *auditCapsule) registerURL(value *url.URL, disposition probeDisposition) {
	a.registerURLWithProvenance(value, disposition, requestProvenancePrivate)
}

func (a *auditCapsule) registerURLWithProvenance(value *url.URL, disposition probeDisposition, provenance requestProvenance) {
	if value == nil {
		a.markUnavailable(AuditReasonProvenanceMissing)
		return
	}
	a.registerSource(value.String(), disposition)
	a.registerSource(value.EscapedPath(), disposition)
	if value.RawPath != "" {
		a.registerSource(value.RawPath, disposition)
	}
	if value.RawQuery != "" {
		// A signed query is credential material as a whole. Individual decoded
		// values and their actual encoded representations are retained too.
		a.registerSource(value.RawQuery, probeSecret)
		query, err := url.ParseQuery(value.RawQuery)
		if err != nil {
			a.markUnavailable(AuditReasonEncodingUnsupported)
			return
		}
		for key, values := range query {
			for _, item := range values {
				if provenance.isFixedPublicControl(key, item, values) {
					continue
				}
				a.registerSource(item, probeSecret)
				a.registerLiteral(url.QueryEscape(item), probeSecret)
			}
		}
	}
}

func (p requestProvenance) isFixedPublicControl(key, value string, values []string) bool {
	if p != requestProvenanceDiagnosticsFixedControls || len(values) != 1 {
		return false
	}
	return (key == "status" && value == "running") || (key == "limit" && value == "100")
}

func (a *auditCapsule) registerSource(value string, disposition probeDisposition) {
	if value == "" {
		return
	}
	if !utf8.ValidString(value) {
		a.markUnavailable(AuditReasonEncodingUnsupported)
		return
	}
	// Reject an oversized source before allocating escaped variants of it.
	a.mu.Lock()
	canFit := a.available && len(value) <= maxAuditMatcherBytes-a.matcherBytes
	a.mu.Unlock()
	if !canFit {
		a.markUnavailable(AuditReasonMatcherLimit)
		return
	}
	variants := []string{value, url.PathEscape(value), url.QueryEscape(value)}
	if encoded, err := json.Marshal(value); err == nil && len(encoded) >= 2 {
		variants = append(variants, string(encoded[1:len(encoded)-1]))
	} else {
		a.markUnavailable(AuditReasonEncodingUnsupported)
		return
	}
	for _, variant := range variants {
		a.registerLiteral(variant, disposition)
	}
}

func (a *auditCapsule) registerLiteral(value string, disposition probeDisposition) {
	if value == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.available {
		return
	}
	if index, ok := a.probeIndex[value]; ok {
		if disposition > a.probes[index].disposition {
			a.probes[index].disposition = disposition
		}
		return
	}
	if len(a.probes) == maxAuditProbes || len(value) > maxAuditMatcherBytes-a.matcherBytes {
		a.available = false
		a.unavailableFor = AuditReasonMatcherLimit
		return
	}
	a.probeIndex[value] = len(a.probes)
	a.probes = append(a.probes, auditProbe{value: value, disposition: disposition})
	a.matcherBytes += len(value)
}

func (a *auditCapsule) markUnavailable(reason AuditReason) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.available {
		a.available = false
		a.unavailableFor = reason
	}
}

func (a *auditCapsule) isAvailable() bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.available
}

func (a *auditCapsule) snapshot() ([]auditProbe, AuditReason, bool) {
	return a.snapshotWithWork(nil)
}

// snapshotWithWork includes capsule-lock contention in the finalizer budget.
// It polls TryLock rather than leaving a blocked lock waiter behind after a
// deadline.  TryLock deliberately happens before the deadline check: a free
// mutex remains usable at the boundary, and later audit work still fails
// closed if its budget has expired.
func (a *auditCapsule) snapshotWithWork(work *auditWork) ([]auditProbe, AuditReason, bool) {
	probes, _, reason, ok := a.snapshotStateWithWork(work)
	return probes, reason, ok
}

// snapshotStateWithWork is the bounded capsule read shared by finalization
// checks. It copies every field needed after unlock so no later path can wait
// indefinitely on capsule.mu or race a concurrent registration.
func (a *auditCapsule) snapshotStateWithWork(work *auditWork) ([]auditProbe, []FaultProfile, AuditReason, bool) {
	if a == nil {
		return nil, nil, AuditReasonCapsuleMissing, false
	}
	for !a.mu.TryLock() {
		if work != nil && (!time.Now().Before(work.deadline) || work.remaining == 0) {
			return nil, nil, AuditReasonDeadline, false
		}
		time.Sleep(time.Millisecond)
	}
	defer a.mu.Unlock()
	if !a.available {
		return nil, nil, a.unavailableFor, false
	}
	return append([]auditProbe(nil), a.probes...), append([]FaultProfile(nil), a.requiredFaults...), "", true
}

type provenanceSpan struct {
	start int
	end   int
	fixed bool // retained for bounded internal test fixtures.
	kind  provenance
}

type decodedToken struct {
	value string
	fixed bool // retained for bounded internal test fixtures.
	kind  provenance
}

type auditDocument struct {
	raw     []byte
	spans   []provenanceSpan
	decoded []decodedToken
}

type provenanceBuilder struct {
	b     strings.Builder
	spans []provenanceSpan
}

func (b *provenanceBuilder) fixed(value string)   { b.append(value, provenanceFixed) }
func (b *provenanceBuilder) dynamic(value string) { b.append(value, provenanceDynamic) }

func (b *provenanceBuilder) append(value string, kind provenance) {
	start := b.b.Len()
	b.b.WriteString(value)
	if value != "" {
		b.spans = append(b.spans, provenanceSpan{start: start, end: b.b.Len(), fixed: kind == provenanceFixed, kind: kind})
	}
}

func (b *provenanceBuilder) document() auditDocument {
	return auditDocument{raw: []byte(b.b.String()), spans: b.spans}
}

type auditWork struct {
	deadline  time.Time
	remaining int // negative is deadline-only; finite values make tests deterministic.
}

func newAuditWork(deadline time.Time) *auditWork {
	return &auditWork{deadline: deadline, remaining: -1}
}

func newAuditWorkBudget(deadline time.Time, remaining int) *auditWork {
	return &auditWork{deadline: deadline, remaining: remaining}
}

func (w *auditWork) allow() bool {
	if w == nil || w.remaining == 0 || time.Now().After(w.deadline) {
		return false
	}
	if w.remaining > 0 {
		w.remaining--
	}
	return true
}

func auditDocuments(capsule *auditCapsule, deadline time.Time, documents ...auditDocument) (AuditStatus, AuditReason) {
	return auditDocumentsWithWork(capsule, newAuditWork(deadline), documents...)
}

func auditDocumentsWithWork(capsule *auditCapsule, work *auditWork, documents ...auditDocument) (AuditStatus, AuditReason) {
	probes, reason, ok := capsule.snapshotWithWork(work)
	if !ok {
		return AuditUnavailable, reason
	}
	for _, probe := range probes {
		if !work.allow() {
			return AuditUnavailable, AuditReasonDeadline
		}
		for _, document := range documents {
			raw, available := unsafeRawMatch(document, probe.value, work)
			if !available {
				return AuditUnavailable, AuditReasonDeadline
			}
			decoded, available := unsafeDecodedMatch(document, probe.value, work)
			if !available {
				return AuditUnavailable, AuditReasonDeadline
			}
			if origin := combinedProvenance(raw, decoded); origin != provenanceFixed {
				switch origin {
				case provenanceSensitive:
					return AuditFailed, AuditReasonSensitiveValue
				case provenanceUnknown:
					return AuditUnavailable, AuditReasonProvenanceMissing
				default:
					return AuditUnavailable, AuditReasonDynamicCollision
				}
			}
		}
	}
	if !work.allow() {
		return AuditUnavailable, AuditReasonDeadline
	}
	return AuditPassed, ""
}

func combinedProvenance(left, right provenance) provenance {
	if left == provenanceSensitive || right == provenanceSensitive {
		return provenanceSensitive
	}
	if left == provenanceUnknown || right == provenanceUnknown {
		return provenanceUnknown
	}
	if left == provenanceDynamic || right == provenanceDynamic {
		return provenanceDynamic
	}
	return provenanceFixed
}

func unsafeRawMatch(document auditDocument, probe string, work *auditWork) (provenance, bool) {
	if probe == "" {
		return provenanceFixed, true
	}
	span := 0
	for offset := 0; offset <= len(document.raw)-len(probe); {
		if !work.allow() {
			return provenanceUnknown, false
		}
		matched := bytes.Index(document.raw[offset:], []byte(probe))
		if matched < 0 {
			return provenanceFixed, true
		}
		start := offset + matched
		end := start + len(probe)
		for span < len(document.spans) && document.spans[span].end <= start {
			span++
		}
		for index := span; index < len(document.spans) && document.spans[index].start < end; index++ {
			if !work.allow() {
				return provenanceUnknown, false
			}
			if origin := document.spans[index].origin(); origin != provenanceFixed {
				return origin, true
			}
		}
		offset = start + 1
	}
	return provenanceFixed, true
}

func unsafeDecodedMatch(document auditDocument, probe string, work *auditWork) (provenance, bool) {
	for _, token := range document.decoded {
		if !work.allow() {
			return provenanceUnknown, false
		}
		if origin := token.origin(); origin != provenanceFixed && strings.Contains(token.value, probe) {
			return origin, true
		}
	}
	return provenanceFixed, true
}

func (s provenanceSpan) origin() provenance {
	if s.kind != provenanceUnknown || !s.fixed {
		return s.kind
	}
	return provenanceFixed
}

func (t decodedToken) origin() provenance {
	if t.kind != provenanceUnknown || !t.fixed {
		return t.kind
	}
	return provenanceFixed
}

type jsonProvenanceParser struct {
	raw      []byte
	position int
	allFixed bool
	document auditDocument
	work     *auditWork
	stopped  bool
}

func parseJSONProvenance(raw []byte, allFixed bool, work *auditWork) (auditDocument, AuditReason, bool) {
	parser := jsonProvenanceParser{raw: raw, allFixed: allFixed, document: auditDocument{raw: raw}, work: work}
	if !parser.value("") {
		return auditDocument{}, parser.reason(), false
	}
	parser.space()
	if parser.stopped || parser.position != len(raw) || !parser.allow() {
		return auditDocument{}, parser.reason(), false
	}
	return parser.document, "", true
}

func (p *jsonProvenanceParser) allow() bool {
	if p.stopped || !p.work.allow() {
		p.stopped = true
		return false
	}
	return true
}

func (p *jsonProvenanceParser) reason() AuditReason {
	if p.stopped {
		return AuditReasonDeadline
	}
	return AuditReasonProvenanceMissing
}

func (p *jsonProvenanceParser) value(path string) bool {
	if !p.allow() {
		return false
	}
	p.space()
	if p.stopped || p.position >= len(p.raw) {
		return false
	}
	switch p.raw[p.position] {
	case '{':
		return p.object(path)
	case '[':
		return p.array(path)
	case '"':
		start, end, decoded, ok := p.stringToken()
		if !ok {
			return false
		}
		kind := provenanceDynamic
		if fixedReportString(path, decoded) {
			kind = provenanceFixed
		}
		p.document.spans = append(p.document.spans, provenanceSpan{start: start + 1, end: end - 1, fixed: kind == provenanceFixed, kind: kind})
		p.document.decoded = append(p.document.decoded, decodedToken{value: decoded, fixed: kind == provenanceFixed, kind: kind})
		return true
	default:
		start := p.position
		for p.position < len(p.raw) && !strings.ContainsRune(" \t\r\n,]}", rune(p.raw[p.position])) {
			if !p.allow() {
				return false
			}
			p.position++
		}
		if start == p.position {
			return false
		}
		kind := provenanceDynamic
		if fixedReportScalar(path, string(p.raw[start:p.position])) {
			kind = provenanceFixed
		}
		p.document.spans = append(p.document.spans, provenanceSpan{start: start, end: p.position, fixed: kind == provenanceFixed, kind: kind})
		return true
	}
}

func (p *jsonProvenanceParser) object(path string) bool {
	p.position++
	p.space()
	if p.take('}') {
		return true
	}
	for {
		if !p.allow() {
			return false
		}
		start, end, key, ok := p.stringToken()
		if !ok {
			return false
		}
		kind := provenanceForJSONKey(path, key)
		p.document.spans = append(p.document.spans, provenanceSpan{start: start + 1, end: end - 1, fixed: kind == provenanceFixed, kind: kind})
		p.document.decoded = append(p.document.decoded, decodedToken{value: key, fixed: kind == provenanceFixed, kind: kind})
		p.space()
		if !p.take(':') {
			return false
		}
		child := key
		if path != "" {
			child = path + "." + key
		}
		if !p.value(child) {
			return false
		}
		p.space()
		if p.take('}') {
			return true
		}
		if !p.take(',') {
			return false
		}
		p.space()
	}
}

func (p *jsonProvenanceParser) array(path string) bool {
	p.position++
	p.space()
	if p.take(']') {
		return true
	}
	itemPath := path + ".*"
	for {
		if !p.allow() {
			return false
		}
		if !p.value(itemPath) {
			return false
		}
		p.space()
		if p.take(']') {
			return true
		}
		if !p.take(',') {
			return false
		}
	}
}

func (p *jsonProvenanceParser) stringToken() (int, int, string, bool) {
	if p.position >= len(p.raw) || p.raw[p.position] != '"' {
		return 0, 0, "", false
	}
	start := p.position
	p.position++
	escaped := false
	for p.position < len(p.raw) {
		if !p.allow() {
			return 0, 0, "", false
		}
		character := p.raw[p.position]
		p.position++
		if escaped {
			escaped = false
			continue
		}
		if character == '\\' {
			escaped = true
			continue
		}
		if character == '"' {
			value, err := strconv.Unquote(string(p.raw[start:p.position]))
			return start, p.position, value, err == nil
		}
	}
	return 0, 0, "", false
}

func (p *jsonProvenanceParser) space() {
	for p.position < len(p.raw) && strings.ContainsRune(" \t\r\n", rune(p.raw[p.position])) {
		if !p.allow() {
			return
		}
		p.position++
	}
}

func (p *jsonProvenanceParser) take(character byte) bool {
	if p.position >= len(p.raw) || p.raw[p.position] != character {
		return false
	}
	p.position++
	return true
}
