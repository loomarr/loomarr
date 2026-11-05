package proposal

// DecisionTrace is bounded, immutable evidence from one original proposal run.
// Callers receive value copies and must not use it as current channel evidence.
type DecisionTrace struct {
	Version                 int                 `json:"version"`
	Candidates              []DecisionCandidate `json:"candidates"`
	SurfacedTotal           int                 `json:"surfacedTotal"`
	RecordedTotal           int                 `json:"recordedTotal"`
	Truncated               bool                `json:"truncated"`
	WindowsCompleted        int                 `json:"windowsCompleted,omitempty"`
	SourceQueriesDispatched int                 `json:"sourceQueriesDispatched,omitempty"`
	Terminal                string              `json:"terminal,omitempty"`
}

// Clone returns a copy that shares no candidate storage with the receiver.
func (t DecisionTrace) Clone() DecisionTrace {
	t.Candidates = append([]DecisionCandidate(nil), t.Candidates...)
	return t
}

type DecisionCandidate struct {
	Key         string            `json:"key"`
	Name        string            `json:"name,omitempty"`
	Source      string            `json:"source,omitempty"`
	Ownership   string            `json:"ownership"`
	Rank        RankTuple         `json:"rank,omitempty"`
	Constraints ConstraintMatches `json:"constraints,omitempty"`
	Disposition string            `json:"disposition"`
	Reason      string            `json:"reason"`
}

// ConstraintMatches identifies which safe intent categories contributed at
// least one grounded term to Relevance. It never copies caller-authored terms.
type ConstraintMatches struct {
	Request     bool `json:"request,omitempty"`
	Tone        bool `json:"tone,omitempty"`
	Era         bool `json:"era,omitempty"`
	MustInclude bool `json:"mustInclude,omitempty"`
	MustExclude bool `json:"mustExclude,omitempty"`
	Refine      bool `json:"refine,omitempty"`
}

// Any reports whether at least one intent category contributed a grounded term.
func (m ConstraintMatches) Any() bool {
	return m.Request || m.Tone || m.Era || m.MustInclude || m.MustExclude || m.Refine
}

// RankTuple is the exact lexicographic ordering tuple. It is not a scalar score.
type RankTuple struct {
	Relevance  int    `json:"relevance"`
	Preference int    `json:"preference"`
	Novelty    int    `json:"novelty"`
	TieKey     string `json:"tieKey"`
}
