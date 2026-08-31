//go:build eval

package eval

import (
	"context"
	"sync"
)

var (
	semanticJudgeAccepted = JudgeScores{
		Overall: 0.95, Relevance: 0.95, Serendipity: 0.90,
		Reason: "Typed JudgeEvidence satisfied semantic validation.",
	}
	semanticJudgeRejected = JudgeScores{
		Overall: 0.10, Relevance: 0.10, Serendipity: 0.10,
		Reason: "Typed JudgeEvidence failed semantic validation.",
	}
)

// semanticRecordingJudge is the shared hermetic test double for the public
// Judge.Score seam. Certification tests validate the typed evidence Runner
// supplies here; they never recover that contract by parsing a model prompt.
type semanticRecordingJudge struct {
	mu       sync.Mutex
	validate func(JudgeEvidence) error
	calls    int
	observed []JudgeEvidence
	errors   []error
}

func newSemanticRecordingJudge(validate func(JudgeEvidence) error) *semanticRecordingJudge {
	return &semanticRecordingJudge{validate: validate}
}

func (j *semanticRecordingJudge) Score(_ context.Context, evidence JudgeEvidence) (JudgeScores, error) {
	j.mu.Lock()
	j.calls++
	j.observed = append(j.observed, evidence)
	validate := j.validate
	j.mu.Unlock()

	if validate != nil {
		if err := validate(evidence); err != nil {
			j.mu.Lock()
			j.errors = append(j.errors, err)
			j.mu.Unlock()
			return semanticJudgeRejected, nil
		}
	}
	return semanticJudgeAccepted, nil
}

func (j *semanticRecordingJudge) CallCount() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.calls
}

func (j *semanticRecordingJudge) Evidence() []JudgeEvidence {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]JudgeEvidence(nil), j.observed...)
}

func (j *semanticRecordingJudge) ValidationErrors() []error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]error(nil), j.errors...)
}
