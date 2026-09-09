package playoutcert

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestPublicationWorkDowngradeRejectsMidFinalizationWithoutMutation(t *testing.T) {
	state := newPublicationState(false, nil)
	state.sealed = true
	state.finalAttempt = &publicationAttempt{done: make(chan struct{})}
	report := Report{Failures: []string{"existing"}, FaultProfiles: []FaultQualification{{Status: "qualified", Outcome: "ok"}}, publication: state}
	before := report
	before.Failures = append([]string(nil), report.Failures...)
	before.FaultProfiles = append([]FaultQualification(nil), report.FaultProfiles...)
	if err := DowngradePublication(&report, PublicationDowngradeCleanupFailed); !errors.Is(err, errPublicationUnavailable) {
		t.Fatalf("downgrade error = %v; want unavailable", err)
	}
	if !reflect.DeepEqual(report, before) {
		t.Fatalf("mid-finalization downgrade changed report: %#v", report)
	}
}

func TestPublicationWorkDowngradeBeforeFinalizationInvalidatesEligibility(t *testing.T) {
	state := newPublicationState(false, nil)
	report := Report{FaultProfiles: []FaultQualification{{Status: "qualified", Outcome: "ok"}}, publication: state}
	digest, err := diagnosticReportDigest(report)
	if err != nil {
		t.Fatal(err)
	}
	state.sealed = true
	state.eligible = true
	state.reportDigest = digest
	if err := DowngradePublication(&report, PublicationDowngradeCleanupFailed); err != nil {
		t.Fatalf("downgrade: %v", err)
	}
	if state.eligible || report.FaultProfiles[0].Status != "unavailable" || report.FaultProfiles[0].Outcome != "cleanup_failed" {
		t.Fatalf("cleanup did not invalidate eligibility and qualified rows: %#v", report)
	}
}

func TestPublicationWorkHeldLockHonorsDeadline(t *testing.T) {
	state := newPublicationState(false, nil)
	report := Report{publication: state}
	state.mu.Lock()
	release := make(chan struct{})
	go func() { <-release; state.mu.Unlock() }()
	publication, err := finalizePublicationWithDeadline(report, time.Now().Add(5*time.Millisecond))
	close(release)
	if !errors.Is(err, errPublicationUnavailable) || publication.AuditStatus() != AuditUnavailable {
		t.Fatalf("held-lock finalization = %#v, %v", publication, err)
	}
}

func TestPublicationWorkFinalizerWaitHonorsDeadline(t *testing.T) {
	state := newPublicationState(false, nil)
	state.finalAttempt = &publicationAttempt{done: make(chan struct{})}
	publication, err := finalizePublicationWithDeadline(Report{publication: state}, time.Now().Add(5*time.Millisecond))
	if !errors.Is(err, errPublicationUnavailable) || publication.AuditStatus() != AuditUnavailable {
		t.Fatalf("finalizer wait = %#v, %v", publication, err)
	}
}

func TestPublicationWorkOwnerPastDeadlinePublishesTerminalResult(t *testing.T) {
	capsule := &auditCapsule{available: true}
	state := newPublicationState(false, capsule)
	report := Report{publication: state}
	digest, err := diagnosticReportDigest(report)
	if err != nil {
		t.Fatal(err)
	}
	state.sealed = true
	state.reportDigest = digest

	// Holding the capsule lets the owner install its attempt, then consume its
	// deadline before it can audit.  Releasing it must still let that same owner
	// publish a terminal result for every later caller.
	capsule.mu.Lock()
	defer capsule.mu.Unlock()
	ownerResult := make(chan struct {
		publication Publication
		err         error
	}, 1)
	go func() {
		publication, ownerErr := finalizePublicationWithDeadline(report, time.Now().Add(25*time.Millisecond))
		ownerResult <- struct {
			publication Publication
			err         error
		}{publication, ownerErr}
	}()
	for until := time.Now().Add(time.Second); ; {
		state.mu.Lock()
		started := state.finalAttempt != nil
		state.mu.Unlock()
		if started {
			break
		}
		if time.Now().After(until) {
			t.Fatal("owner did not install finalization attempt")
		}
		time.Sleep(time.Millisecond)
	}
	beforeDowngrade := report
	if downgradeErr := DowngradePublication(&report, PublicationDowngradeCleanupFailed); !errors.Is(downgradeErr, errPublicationUnavailable) || !reflect.DeepEqual(report, beforeDowngrade) {
		t.Fatalf("downgrade after owner start = %v, %#v; want unavailable without mutation", downgradeErr, report)
	}
	time.Sleep(30 * time.Millisecond)
	capsule.mu.Unlock()
	defer capsule.mu.Lock()
	owner := <-ownerResult
	if !errors.Is(owner.err, errPublicationUnavailable) || owner.publication.AuditStatus() != AuditUnavailable {
		t.Fatalf("owner result = %#v, %v; want terminal unavailable", owner.publication, owner.err)
	}
	reused, reuseErr := finalizePublicationWithDeadline(report, time.Now().Add(time.Second))
	if !errors.Is(reuseErr, errPublicationUnavailable) || !reflect.DeepEqual(reused, owner.publication) {
		t.Fatalf("reused result = %#v, %v; want %#v, unavailable", reused, reuseErr, owner.publication)
	}
}

func TestPublicationWorkEligibleQualificationCheckHonorsCapsuleDeadline(t *testing.T) {
	capsule := &auditCapsule{available: true}
	state := newPublicationState(true, capsule)
	report := Report{publication: state}
	digest, err := diagnosticReportDigest(report)
	if err != nil {
		t.Fatal(err)
	}
	state.sealed = true
	state.eligible = true
	state.reportDigest = digest

	capsule.mu.Lock()
	result := make(chan struct {
		publication Publication
		err         error
	}, 1)
	go func() {
		publication, finalizeErr := finalizePublicationWithDeadline(report, time.Now().Add(25*time.Millisecond))
		result <- struct {
			publication Publication
			err         error
		}{publication, finalizeErr}
	}()
	owner := <-result
	capsule.mu.Unlock()
	if !errors.Is(owner.err, errPublicationUnavailable) || owner.publication.AuditStatus() != AuditUnavailable {
		t.Fatalf("eligible held-capsule result = %#v, %v; want unavailable", owner.publication, owner.err)
	}
}

func TestPublicationWorkConcurrentCallersConvergeOnOwnerResult(t *testing.T) {
	state := newPublicationState(false, &auditCapsule{available: true})
	report := Report{publication: state}
	digest, err := diagnosticReportDigest(report)
	if err != nil {
		t.Fatal(err)
	}
	state.sealed = true
	state.reportDigest = digest

	type result struct {
		publication Publication
		err         error
	}
	results := make(chan result, 8)
	start := make(chan struct{})
	for range 8 {
		go func() {
			<-start
			publication, finalizeErr := finalizePublicationWithDeadline(report, time.Now().Add(time.Second))
			results <- result{publication, finalizeErr}
		}()
	}
	close(start)
	var expected result
	for index := 0; index < 8; index++ {
		actual := <-results
		if actual.err != nil || actual.publication.AuditStatus() != AuditPassed {
			t.Fatalf("caller %d = %#v, %v; want audited result", index, actual.publication, actual.err)
		}
		if index == 0 {
			expected = actual
		} else if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("caller %d = %#v, %v; want %#v, %v", index, actual.publication, actual.err, expected.publication, expected.err)
		}
	}
}

func TestPublicationWorkAccessorsCopyFrozenBytes(t *testing.T) {
	publication := Publication{json: []byte("json"), summary: []byte("summary")}
	jsonBytes := publication.JSON()
	summaryBytes := publication.Summary()
	jsonBytes[0] = 'J'
	summaryBytes[0] = 'S'
	if string(publication.JSON()) != "json" || string(publication.Summary()) != "summary" {
		t.Fatalf("accessor mutation changed publication: %#v", publication)
	}
}
