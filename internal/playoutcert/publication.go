package playoutcert

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"sync"
	"time"
)

const (
	maxPublicationOutputBytes = 8 << 20
	publicationDeadline       = 30 * time.Second
)

// AuditStatus is the fixed public result of auditing exact publication bytes.
type AuditStatus string

const (
	AuditMissing     AuditStatus = "missing"
	AuditPassed      AuditStatus = "passed"
	AuditFailed      AuditStatus = "failed"
	AuditUnavailable AuditStatus = "unavailable"
)

// AuditReason is deliberately fixed vocabulary: errors never contain matched
// bytes or source material from the private capsule.
type AuditReason string

const (
	AuditReasonNotFinalized        AuditReason = "publication_not_finalized"
	AuditReasonCapsuleMissing      AuditReason = "audit_capsule_missing"
	AuditReasonRunProofMissing     AuditReason = "run_proof_missing"
	AuditReasonReportMutated       AuditReason = "report_mutated"
	AuditReasonSensitiveValue      AuditReason = "sensitive_value_exposed"
	AuditReasonDynamicCollision    AuditReason = "dynamic_value_collision"
	AuditReasonMatcherLimit        AuditReason = "matcher_limit_exceeded"
	AuditReasonOutputLimit         AuditReason = "output_limit_exceeded"
	AuditReasonDeadline            AuditReason = "finalization_deadline_exceeded"
	AuditReasonEncodingUnsupported AuditReason = "encoding_unsupported"
	AuditReasonProvenanceMissing   AuditReason = "provenance_missing"
)

// PublicationDowngrade is a closed set of post-run, pre-publication failures.
type PublicationDowngrade string

const PublicationDowngradeCleanupFailed PublicationDowngrade = "isolated_cleanup_failed"

// PublicationVerdict is frozen with the exact output bytes.
type PublicationVerdict string

const (
	VerdictCertified   PublicationVerdict = "certified"
	VerdictUncertified PublicationVerdict = "uncertified"
	VerdictUnavailable PublicationVerdict = "unpublishable"
)

var errPublicationUnavailable = errors.New("playout certification publication unavailable")

type publicationState struct {
	mu               sync.Mutex
	audit            *auditCapsule
	certifyRequested bool
	sealed           bool
	eligible         bool
	reportDigest     [sha256.Size]byte
	downgrade        PublicationDowngrade
	finalAttempt     *publicationAttempt
}

// publicationAttempt is installed while holding publicationState.mu, then its
// result is published by closing done.  Closing the channel establishes the
// happens-before edge for every waiter, so an owner can always finish without
// reacquiring the state mutex (and without leaving a stranded finalizer).
type publicationAttempt struct {
	done        chan struct{}
	publication Publication
	err         error
}

func newPublicationState(certify bool, audit *auditCapsule) *publicationState {
	return &publicationState{audit: audit, certifyRequested: certify}
}

func sealRunReport(report *Report, eligible bool) {
	if report == nil || report.publication == nil {
		return
	}
	report.Certified = false
	report.AuditStatus = AuditMissing
	report.AuditReason = ""
	digest, err := diagnosticReportDigest(*report)
	state := report.publication
	state.mu.Lock()
	defer state.mu.Unlock()
	state.eligible = eligible && err == nil
	state.sealed = err == nil
	state.reportDigest = digest
}

// DowngradePublication records an allowed cleanup failure without blessing
// arbitrary public-field edits. It cannot be applied after finalization has
// begun: the finalizer owns a snapshot, and cleanup must precede that snapshot.
func DowngradePublication(report *Report, reason PublicationDowngrade) error {
	if report == nil || report.publication == nil || reason != PublicationDowngradeCleanupFailed {
		return errPublicationUnavailable
	}
	// Do the cheap, fail-closed size walk before taking the publication lock.
	// This prevents arbitrary public input from making cleanup hold that lock
	// while JSON marshaling a report which can never be published.
	if !boundedReportInput(*report, time.Now().Add(publicationDeadline)) {
		return errPublicationUnavailable
	}
	state := report.publication
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.finalAttempt != nil || !state.sealed {
		return errPublicationUnavailable
	}
	digest, err := diagnosticReportDigest(*report)
	if err != nil || digest != state.reportDigest {
		state.sealed = false
		state.eligible = false
		return errPublicationUnavailable
	}
	if !slices.Contains(report.Failures, string(reason)) {
		report.Failures = append(report.Failures, string(reason))
	}
	report.Certified = false
	for index := range report.FaultProfiles {
		if report.FaultProfiles[index].Status == "qualified" {
			report.FaultProfiles[index].Status = "unavailable"
			report.FaultProfiles[index].Outcome = "cleanup_failed"
		}
	}
	state.eligible = false
	state.downgrade = reason
	state.reportDigest, err = diagnosticReportDigest(*report)
	state.sealed = err == nil
	if err != nil {
		return errPublicationUnavailable
	}
	return nil
}

// Publication owns immutable, already-audited bytes. Byte accessors return
// copies so callers cannot alter the frozen artifact or summary.
type Publication struct {
	json       []byte
	summary    []byte
	audit      AuditStatus
	verdict    PublicationVerdict
	exitStatus int
}

func (p Publication) JSON() []byte                { return append([]byte(nil), p.json...) }
func (p Publication) Summary() []byte             { return append([]byte(nil), p.summary...) }
func (p Publication) AuditStatus() AuditStatus    { return p.audit }
func (p Publication) Verdict() PublicationVerdict { return p.verdict }
func (p Publication) ExitStatus() int             { return p.exitStatus }
func (p Publication) Publishable() bool           { return len(p.json) != 0 && len(p.summary) != 0 }

// FinalizePublication is the sole path to detailed report bytes. It verifies
// the sealed run snapshot, derives certification from private eligibility, and
// audits the exact JSON and summary before freezing either one.
func FinalizePublication(report Report) (Publication, error) {
	return finalizePublicationWithDeadline(report, time.Now().Add(publicationDeadline))
}

type publicationSnapshot struct {
	audit            *auditCapsule
	certifyRequested bool
	sealed           bool
	eligible         bool
	reportDigest     [sha256.Size]byte
}

// finalizePublicationWithDeadline keeps the expensive marshal/parser/matcher
// work outside the state lock. Concurrent callers wait only until their own
// deadline, while the owner still produces the one cached immutable result.
func finalizePublicationWithDeadline(report Report, deadline time.Time) (Publication, error) {
	if report.publication == nil {
		return fallbackPublication(nil, AuditMissing, AuditReasonCapsuleMissing, deadline)
	}
	state := report.publication
	if !lockPublicationUntil(&state.mu, deadline) {
		return unavailablePublication()
	}
	if attempt := state.finalAttempt; attempt != nil {
		state.mu.Unlock()
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return Publication{audit: AuditUnavailable, verdict: VerdictUnavailable, exitStatus: 1}, errPublicationUnavailable
		}
		select {
		case <-attempt.done:
			return clonePublication(attempt.publication), attempt.err
		case <-time.After(remaining):
			return unavailablePublication()
		}
	}
	attempt := &publicationAttempt{done: make(chan struct{})}
	state.finalAttempt = attempt
	snapshot := publicationSnapshot{state.audit, state.certifyRequested, state.sealed, state.eligible, state.reportDigest}
	state.mu.Unlock()
	finish := func(publication Publication, err error) (Publication, error) {
		attempt.publication = clonePublication(publication)
		attempt.err = err
		close(attempt.done)
		return clonePublication(publication), err
	}
	if !snapshot.sealed {
		publication, err := fallbackPublication(snapshot.audit, AuditMissing, AuditReasonRunProofMissing, deadline)
		return finish(publication, err)
	}
	if !boundedReportInput(report, deadline) {
		publication, err := fallbackPublication(snapshot.audit, AuditUnavailable, AuditReasonOutputLimit, deadline)
		return finish(publication, err)
	}
	digest, err := diagnosticReportDigest(report)
	if err != nil || digest != snapshot.reportDigest {
		publication, fallbackErr := fallbackPublication(snapshot.audit, AuditUnavailable, AuditReasonReportMutated, deadline)
		return finish(publication, fallbackErr)
	}
	candidate := report
	candidate.SchemaVersion = SchemaVersion
	candidate.Certified = snapshot.eligible && len(candidate.Failures) == 0 && requiredQualificationsPassed(candidate, snapshot.audit, newAuditWork(deadline))
	candidate.AuditStatus = AuditPassed
	candidate.AuditReason = ""
	jsonBytes, err := marshalDiagnosticReport(candidate)
	if err != nil {
		publication, fallbackErr := fallbackPublication(snapshot.audit, AuditUnavailable, AuditReasonEncodingUnsupported, deadline)
		return finish(publication, fallbackErr)
	}
	jsonBytes = append(jsonBytes, '\n')
	summaryDocument := renderReportSummary(candidate)
	if len(jsonBytes) > maxPublicationOutputBytes-len(summaryDocument.raw) {
		publication, fallbackErr := fallbackPublication(snapshot.audit, AuditUnavailable, AuditReasonOutputLimit, deadline)
		return finish(publication, fallbackErr)
	}
	work := newAuditWork(deadline)
	jsonDocument, parseReason, ok := parseJSONProvenance(jsonBytes, false, work)
	if !ok {
		publication, fallbackErr := fallbackPublication(snapshot.audit, AuditUnavailable, parseReason, deadline)
		return finish(publication, fallbackErr)
	}
	status, reason := auditDocumentsWithWork(snapshot.audit, work, jsonDocument, summaryDocument)
	if status != AuditPassed {
		publication, fallbackErr := fallbackPublication(snapshot.audit, status, reason, deadline)
		return finish(publication, fallbackErr)
	}
	exitStatus := 0
	if len(candidate.Failures) != 0 || (snapshot.certifyRequested && !candidate.Certified) {
		exitStatus = 1
	}
	verdict := VerdictUncertified
	if candidate.Certified {
		verdict = VerdictCertified
	}
	return finish(Publication{json: jsonBytes, summary: summaryDocument.raw, audit: AuditPassed, verdict: verdict, exitStatus: exitStatus}, nil)
}

func unavailablePublication() (Publication, error) {
	return Publication{audit: AuditUnavailable, verdict: VerdictUnavailable, exitStatus: 1}, errPublicationUnavailable
}

// lockPublicationUntil is deliberately polling rather than spawning a lock
// waiter: FinalizePublication's deadline includes every mutex wait and leaves
// no detached goroutine behind when that deadline expires.
func lockPublicationUntil(mu *sync.Mutex, deadline time.Time) bool {
	for {
		if mu.TryLock() {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(time.Millisecond)
	}
}

func fallbackPublication(capsule *auditCapsule, status AuditStatus, reason AuditReason, deadline time.Time) (Publication, error) {
	minimal := minimalReport{SchemaVersion: SchemaVersion, Certified: false, AuditStatus: status, AuditReason: reason}
	jsonBytes, err := json.Marshal(minimal)
	if err != nil {
		return Publication{audit: AuditUnavailable, verdict: VerdictUnavailable, exitStatus: 1}, errPublicationUnavailable
	}
	jsonBytes = append(jsonBytes, '\n')
	summary := minimalSummary(status, reason)
	if len(jsonBytes) > maxPublicationOutputBytes-len(summary.raw) || time.Now().After(deadline) {
		return Publication{audit: AuditUnavailable, verdict: VerdictUnavailable, exitStatus: 1}, errPublicationUnavailable
	}
	work := newAuditWork(deadline)
	jsonDocument, _, ok := parseJSONProvenance(jsonBytes, true, work)
	if !ok {
		return Publication{audit: AuditUnavailable, verdict: VerdictUnavailable, exitStatus: 1}, errPublicationUnavailable
	}
	if capsule != nil {
		if audited, _ := auditDocumentsWithWork(capsule, work, jsonDocument, summary); audited != AuditPassed {
			return Publication{audit: AuditUnavailable, verdict: VerdictUnavailable, exitStatus: 1}, errPublicationUnavailable
		}
	}
	return Publication{json: jsonBytes, summary: summary.raw, audit: status, verdict: VerdictUncertified, exitStatus: 1}, nil
}

type minimalReport struct {
	SchemaVersion int         `json:"schemaVersion"`
	Certified     bool        `json:"certified"`
	AuditStatus   AuditStatus `json:"auditStatus"`
	AuditReason   AuditReason `json:"auditReason"`
}

func (r Report) MarshalJSON() ([]byte, error) {
	return json.Marshal(minimalReport{SchemaVersion: SchemaVersion, Certified: false, AuditStatus: AuditMissing, AuditReason: AuditReasonNotFinalized})
}

type diagnosticReport Report

func marshalDiagnosticReport(report Report) ([]byte, error) {
	return json.Marshal(diagnosticReport(report))
}

func diagnosticReportDigest(report Report) ([sha256.Size]byte, error) {
	encoded, err := marshalDiagnosticReport(report)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(encoded), nil
}

func requiredQualificationsPassed(report Report, capsule *auditCapsule, work *auditWork) bool {
	_, required, _, ok := capsule.snapshotStateWithWork(work)
	if !ok {
		return false
	}
	for _, profile := range required {
		found := false
		for _, row := range report.FaultProfiles {
			if row.Profile == profile && row.Status == "qualified" {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func clonePublication(publication Publication) Publication {
	publication.json = append([]byte(nil), publication.json...)
	publication.summary = append([]byte(nil), publication.summary...)
	return publication
}

func boundedReportInput(report Report, deadline time.Time) bool {
	remaining := int64(maxPublicationOutputBytes)
	checks := 0
	return boundedJSONValue(reflect.ValueOf(diagnosticReport(report)), &remaining, deadline, 0, &checks)
}

var timeType = reflect.TypeOf(time.Time{})

func boundedJSONValue(value reflect.Value, remaining *int64, deadline time.Time, depth int, checks *int) bool {
	*checks++
	if *checks&1023 == 0 && time.Now().After(deadline) {
		return false
	}
	if depth > 32 || !value.IsValid() {
		return depth <= 32
	}
	if value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return consumeBound(remaining, 4)
		}
		return boundedJSONValue(value.Elem(), remaining, deadline, depth+1, checks)
	}
	if value.Type() == timeType {
		return consumeBound(remaining, 128)
	}
	switch value.Kind() {
	case reflect.String:
		length := int64(value.Len())
		if length > (1<<62)/6 {
			return false
		}
		return consumeBound(remaining, length*6+2)
	case reflect.Bool:
		return consumeBound(remaining, 5)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return consumeBound(remaining, 64)
	case reflect.Slice, reflect.Array:
		if !consumeCollectionBound(remaining, value.Len()) {
			return false
		}
		for index := 0; index < value.Len(); index++ {
			if !boundedJSONValue(value.Index(index), remaining, deadline, depth+1, checks) {
				return false
			}
		}
		return true
	case reflect.Map:
		if value.IsNil() {
			return consumeBound(remaining, 4)
		}
		if !consumeCollectionBound(remaining, value.Len()) {
			return false
		}
		iterator := value.MapRange()
		for iterator.Next() {
			if !boundedJSONValue(iterator.Key(), remaining, deadline, depth+1, checks) || !boundedJSONValue(iterator.Value(), remaining, deadline, depth+1, checks) {
				return false
			}
		}
		return true
	case reflect.Struct:
		typeOf := value.Type()
		for index := 0; index < value.NumField(); index++ {
			field := typeOf.Field(index)
			if field.PkgPath != "" || field.Tag.Get("json") == "-" {
				continue
			}
			name := field.Name
			if tag := field.Tag.Get("json"); tag != "" {
				if comma := slices.Index([]byte(tag), ','); comma >= 0 {
					name = tag[:comma]
				} else {
					name = tag
				}
			}
			if !consumeBound(remaining, int64(len(name))*6+4) || !boundedJSONValue(value.Field(index), remaining, deadline, depth+1, checks) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func consumeCollectionBound(remaining *int64, length int) bool {
	if length < 0 || int64(length) > *remaining/2 {
		return false
	}
	return consumeBound(remaining, int64(length)*2+2)
}

func consumeBound(remaining *int64, amount int64) bool {
	if amount < 0 || amount > *remaining {
		return false
	}
	*remaining -= amount
	return true
}
