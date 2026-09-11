package fillercorpus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

const HistoricalInventorySchemaVersion = 4

// HistoricalInventoryEvidence is the non-authorizing identity exposed for a
// valid schema-v4 inventory. It deliberately omits representations and cannot
// be passed to acquisition, review, or preparation APIs.
type HistoricalInventoryEvidence struct {
	SchemaVersion int
	SHA256        string
	SnapshotAt    time.Time
	CaseIDs       []string
}

func DecodeHistoricalInventoryEvidence(reader io.Reader) (HistoricalInventoryEvidence, error) {
	raw, err := io.ReadAll(reader)
	if err != nil {
		return HistoricalInventoryEvidence{}, fmt.Errorf("read historical inventory: %w", err)
	}
	var value Inventory
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return HistoricalInventoryEvidence{}, fmt.Errorf("decode historical inventory: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return HistoricalInventoryEvidence{}, fmt.Errorf("decode historical inventory: trailing JSON value")
	}
	if value.SchemaVersion != HistoricalInventorySchemaVersion {
		return HistoricalInventoryEvidence{}, fmt.Errorf("historical inventory schemaVersion is %d; want %d", value.SchemaVersion, HistoricalInventorySchemaVersion)
	}
	validated := value
	validated.SchemaVersion = InventorySchemaVersion
	caseIDs := make([]string, 0, len(validated.Cases))
	for index := range validated.Cases {
		item := &validated.Cases[index]
		if item.Representation.Soundtrack != (SoundtrackExpectation{}) {
			return HistoricalInventoryEvidence{}, fmt.Errorf("historical inventory case %q contains post-v4 soundtrack authority", item.CaseID)
		}
		item.Representation.Soundtrack = BindInventorySoundtrack(item.Representation, SoundtrackUnknown, SoundtrackEvidenceFirstPartyMetadata, item.MetadataSHA256, "historical schema-v4 inventory omitted soundtrack expectation")
		caseIDs = append(caseIDs, item.CaseID)
	}
	if failures := ValidateInventory(validated); len(failures) != 0 {
		return HistoricalInventoryEvidence{}, fmt.Errorf("invalid historical inventory: %s", strings.Join(failures, "; "))
	}
	return HistoricalInventoryEvidence{
		SchemaVersion: value.SchemaVersion, SHA256: InventorySHA256(raw), SnapshotAt: value.SnapshotAt,
		CaseIDs: caseIDs,
	}, nil
}
