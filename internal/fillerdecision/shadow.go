package fillerdecision

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/filleradmission"
)

// RecordWriter is the persistence seam used by Shadow. Service is the production adapter; tests
// use an in-memory adapter so document construction, evaluation, identity, and persistence are
// exercised through the same interface.
type RecordWriter interface {
	Record(context.Context, Record) error
}

// ShadowObservation is the version-free production input. Extractors supply only the facts and
// operational states they actually observed; Shadow owns every version and durable identity.
type ShadowObservation struct {
	ClipHash    string
	ObservedAt  time.Time
	Evidence    []filleradmission.Evidence
	Operational []filleradmission.OperationalIssue
	Attribution []filleradmission.Attribution
}

// Shadow is the deep production module between extraction and the immutable V63 audit. Its one
// interface versions and hashes the evidence, runs the pure evaluator, and records the result.
type Shadow struct {
	writer          RecordWriter
	evaluator       *filleradmission.Evaluator
	evidenceVersion string
}

func NewShadow(writer RecordWriter, policy filleradmission.Policy, evidenceVersion string) (*Shadow, error) {
	if writer == nil || evidenceVersion == "" {
		return nil, fmt.Errorf("%w: shadow writer and evidence version are required", ErrInvalid)
	}
	evaluator, err := filleradmission.New(policy)
	if err != nil {
		return nil, fmt.Errorf("construct filler admission shadow: %w", err)
	}
	return &Shadow{writer: writer, evaluator: evaluator, evidenceVersion: evidenceVersion}, nil
}

// Observe records exactly one deterministic decision for one observed evidence state. Repeating
// the same observation produces the same id and payload, so a crash after insertion but before the
// pipeline advances is an idempotent retry rather than duplicate Activity.
func (s *Shadow) Observe(ctx context.Context, observation ShadowObservation) error {
	if s == nil || s.writer == nil || s.evaluator == nil || observation.ClipHash == "" || observation.ObservedAt.IsZero() {
		return fmt.Errorf("%w: shadow observation requires clip identity and time", ErrInvalid)
	}
	policyVersion, taxonomyVersion := s.evaluator.PolicyIdentity()
	doc := filleradmission.Document{
		SchemaVersion: filleradmission.SchemaVersion, EvidenceVersion: s.evidenceVersion,
		PolicyVersion: policyVersion, TaxonomyVersion: taxonomyVersion, ClipHash: observation.ClipHash,
		Evidence:    append([]filleradmission.Evidence(nil), observation.Evidence...),
		Operational: append([]filleradmission.OperationalIssue(nil), observation.Operational...),
		Attribution: append([]filleradmission.Attribution(nil), observation.Attribution...),
	}
	payload, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal filler admission evidence: %w", err)
	}
	evidenceDigest := sha256.Sum256(payload)
	evidenceHash := hex.EncodeToString(evidenceDigest[:])
	identityDigest := sha256.Sum256([]byte(observation.ClipHash + "\x00" + evidenceHash + "\x00" +
		observation.ObservedAt.UTC().Format(time.RFC3339Nano)))
	record := Record{
		ID: "shadow-" + hex.EncodeToString(identityDigest[:]), ClipHash: observation.ClipHash,
		EvidenceHash: evidenceHash, EvidenceVersion: doc.EvidenceVersion,
		SchemaVersion: doc.SchemaVersion, PolicyVersion: doc.PolicyVersion,
		TaxonomyVersion: doc.TaxonomyVersion, Result: s.evaluator.Evaluate(doc),
		CreatedAt: observation.ObservedAt.UTC(),
	}
	if err := s.writer.Record(ctx, record); err != nil {
		return fmt.Errorf("record filler admission shadow: %w", err)
	}
	return nil
}
