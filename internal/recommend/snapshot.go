package recommend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"
)

const maxSnapshotSignals = 64

// DecodeSnapshot is the privacy boundary for provider-visible recommendation
// context. Its closed schema deliberately has no identity or viewing-history
// fields, and unknown fields fail rather than being silently forwarded later.
func DecodeSnapshot(raw []byte) (Snapshot, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var snapshot Snapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("snapshot field: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Snapshot{}, fmt.Errorf("snapshot field: trailing JSON value")
	}
	if snapshot.ID == "" || len(snapshot.Signals) > maxSnapshotSignals || len(snapshot.ExistingConcepts) > 32 {
		return Snapshot{}, fmt.Errorf("snapshot field: missing id or too many signals")
	}
	seen := make(map[string]bool, len(snapshot.Signals))
	for _, signal := range snapshot.Signals {
		if signal.ID == "" || signal.Value == "" || seen[signal.ID] || !knownSignalKind(signal.Kind) {
			return Snapshot{}, fmt.Errorf("snapshot field: invalid or duplicate signal")
		}
		seen[signal.ID] = true
	}
	for _, concept := range snapshot.ExistingConcepts {
		if normalizedConceptText(concept.Name) == "" || normalizedConceptText(concept.IntentDescription) == "" {
			return Snapshot{}, fmt.Errorf("snapshot field: invalid existing concept")
		}
	}
	return snapshot, nil
}

func knownSignalKind(kind SignalKind) bool {
	return kind == SignalLibraryGenre || kind == SignalLibraryEra || kind == SignalPreference || kind == SignalSeason || kind == SignalConstraint
}

func normalizedConceptText(value string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}), " ")
}
