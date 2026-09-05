package fillerreview

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const TemporalStructureHoldoutProgrammeInventoryContract = "filler-temporal-structure-programme-inventory-v1"

func loadTemporalStructureHoldoutProgrammeInventory(path, sourceRoot string, plannedAt time.Time) (TemporalStructureHoldoutProgrammeInventory, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return TemporalStructureHoldoutProgrammeInventory{}, "", err
	}
	inventory, err := readStrictJSON[TemporalStructureHoldoutProgrammeInventory](path)
	if err != nil {
		return TemporalStructureHoldoutProgrammeInventory{}, "", err
	}
	if inventory.SchemaVersion != TemporalStructureHoldoutSchemaVersion || inventory.ContractVersion != TemporalStructureHoldoutProgrammeInventoryContract || inventory.GeneratedAt.IsZero() || plannedAt.Before(inventory.GeneratedAt) || len(inventory.Sources) < temporalStructureHoldoutParentSources {
		return TemporalStructureHoldoutProgrammeInventory{}, "", fmt.Errorf("temporal structure holdout programme inventory is incomplete")
	}
	seenIDs, seenPaths, seenSHA := map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}
	seenProvenance := map[string]struct{}{}
	for _, source := range inventory.Sources {
		if source.Provenance.Kind != TemporalStructureSourceProgrammeParent || source.StandaloneRole != "" || source.DurationMS < 120_000 || strings.TrimSpace(source.ID) == "" || !reviewSHA256(source.SHA256) || strings.TrimSpace(source.Path) == "" || strings.TrimSpace(source.Provenance.Authority) == "" || strings.TrimSpace(source.Provenance.Reference) == "" || !reviewSHA256(source.Provenance.MetadataSHA256) || source.Provenance.RetrievedAt.IsZero() || inventory.GeneratedAt.Before(source.Provenance.RetrievedAt) {
			return TemporalStructureHoldoutProgrammeInventory{}, "", fmt.Errorf("temporal structure holdout programme source %q is invalid", source.ID)
		}
		resolved, err := resolveWithin(sourceRoot, source.Path)
		if err != nil {
			return TemporalStructureHoldoutProgrammeInventory{}, "", fmt.Errorf("temporal structure holdout programme source %q path: %w", source.ID, err)
		}
		digest, err := hashFile(resolved)
		if err != nil || digest != source.SHA256 {
			return TemporalStructureHoldoutProgrammeInventory{}, "", fmt.Errorf("temporal structure holdout programme source %q bytes drift", source.ID)
		}
		if _, duplicate := seenIDs[source.ID]; duplicate {
			return TemporalStructureHoldoutProgrammeInventory{}, "", fmt.Errorf("temporal structure holdout programme inventory repeats an id")
		}
		if _, duplicate := seenPaths[source.Path]; duplicate {
			return TemporalStructureHoldoutProgrammeInventory{}, "", fmt.Errorf("temporal structure holdout programme inventory repeats a path")
		}
		if _, duplicate := seenSHA[source.SHA256]; duplicate {
			return TemporalStructureHoldoutProgrammeInventory{}, "", fmt.Errorf("temporal structure holdout programme inventory repeats source bytes")
		}
		provenanceID := source.Provenance.Authority + "\x00" + source.Provenance.Reference
		if _, duplicate := seenProvenance[provenanceID]; duplicate {
			return TemporalStructureHoldoutProgrammeInventory{}, "", fmt.Errorf("temporal structure holdout programme inventory repeats a provenance parent")
		}
		seenIDs[source.ID], seenPaths[source.Path], seenSHA[source.SHA256] = struct{}{}, struct{}{}, struct{}{}
		seenProvenance[provenanceID] = struct{}{}
	}
	return inventory, hashBytes(raw), nil
}
