// Command filler-corpus-rights-review prepares a deterministic, non-authorizing
// worksheet from a frozen source inventory. It never downloads media.
package main

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/loomarr/loomarr/internal/fillercorpus"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-corpus-rights-review", flag.ContinueOnError)
	flags.SetOutput(stderr)
	inventoryPath := flags.String("inventory", "", "frozen source inventory JSON")
	outputPath := flags.String("out", "", "non-authorizing rights worksheet JSON")
	csvOutputPath := flags.String("csv-out", "", "spreadsheet-safe non-authorizing rights worksheet CSV")
	preparedAtText := flags.String("prepared-at", "", "worksheet preparation time in RFC3339 format")
	minItems := flags.Int("min-items", 0, "required minimum worksheet cases")
	maxItems := flags.Int("max-items", 0, "hard worksheet case ceiling")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *inventoryPath == "" || *outputPath == "" || *preparedAtText == "" || *minItems <= 0 || *maxItems < *minItems {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-rights-review: inventory, output, preparation time, and positive min/max item bounds are required")
		return 2
	}
	preparedAt, err := time.Parse(time.RFC3339, *preparedAtText)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-rights-review: parse --prepared-at:", err)
		return 2
	}
	raw, err := os.ReadFile(*inventoryPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-rights-review: read inventory:", err)
		return 1
	}
	inv, err := fillercorpus.DecodeInventoryBytes(raw)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-rights-review: decode inventory:", err)
		return 1
	}
	result, err := prepareWorksheet(inv, sha256Hex(raw), preparedAt, *minItems, *maxItems)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-rights-review:", err)
		return 1
	}
	if err := writeJSON(*outputPath, result); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-rights-review: write worksheet:", err)
		return 1
	}
	if *csvOutputPath != "" {
		if err := writeReviewCSV(*csvOutputPath, result); err != nil {
			_, _ = fmt.Fprintln(stderr, "filler-corpus-rights-review: write CSV worksheet:", err)
			return 1
		}
	}
	_, _ = fmt.Fprintf(stdout, "filler-corpus-rights-review: prepared %d inert review rows for inventory %s\n", len(result.Cases), result.InventorySHA256)
	return 0
}

func writeReviewCSV(path string, sheet fillercorpus.RightsWorksheet) error {
	return writeAtomic(path, func(writer io.Writer) error {
		csvWriter := csv.NewWriter(writer)
		if err := csvWriter.Write(fillercorpus.RightsReviewCSVHeader()); err != nil {
			return err
		}
		for _, row := range sheet.Cases {
			record := append(fillercorpus.ImmutableRightsReviewRecord(row), "", "", "", "", "", "", "")
			if err := csvWriter.Write(record); err != nil {
				return err
			}
		}
		csvWriter.Flush()
		return csvWriter.Error()
	})
}

func prepareWorksheet(inv fillercorpus.Inventory, digest string, preparedAt time.Time, minItems, maxItems int) (fillercorpus.RightsWorksheet, error) {
	if failures := fillercorpus.ValidateInventory(inv); len(failures) != 0 || preparedAt.Before(inv.SnapshotAt) {
		return fillercorpus.RightsWorksheet{}, fmt.Errorf("inventory identity or worksheet time is invalid")
	}
	if len(inv.Cases) < minItems {
		return fillercorpus.RightsWorksheet{}, fmt.Errorf("inventory has %d cases; minimum is %d", len(inv.Cases), minItems)
	}
	cases := append([]fillercorpus.InventoryCase(nil), inv.Cases...)
	sort.Slice(cases, func(i, j int) bool {
		return sha256Hex([]byte(digest+"/"+cases[i].CaseID)) < sha256Hex([]byte(digest+"/"+cases[j].CaseID))
	})
	if len(cases) > maxItems {
		cases = cases[:maxItems]
	}
	result := fillercorpus.RightsWorksheet{
		SchemaVersion: fillercorpus.RightsWorksheetSchemaVersion, InventorySHA256: digest,
		SnapshotAt: inv.SnapshotAt.UTC(), PreparedAt: preparedAt.UTC(), MinItems: minItems, MaxItems: maxItems,
		Instructions: []string{
			"This worksheet is not download authority; blank rows fail closed.",
			"Review the exact item, metadata, selected file, license assertion, rights prose, embedded material, attribution, and non-copyright restrictions.",
			"Set decision to approved only with explicit redistributable=true and a reasoned basis; otherwise set decision to held.",
		},
	}
	seen := map[string]struct{}{}
	for index, item := range cases {
		if _, exists := seen[item.CaseID]; exists {
			return fillercorpus.RightsWorksheet{}, fmt.Errorf("duplicate candidate %s", item.CaseID)
		}
		seen[item.CaseID] = struct{}{}
		row := fillercorpus.RightsReviewRowFromCase(item)
		row.Rank, row.InventorySHA256 = index+1, digest
		result.Cases = append(result.Cases, row)
	}
	return result, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, func(writer io.Writer) error {
		_, err := writer.Write(append(data, '\n'))
		return err
	})
}

func writeAtomic(path string, write func(io.Writer) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".filler-corpus-rights-review-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	ok := false
	defer func() {
		_ = temp.Close()
		if !ok {
			_ = os.Remove(tempName)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	if err := write(temp); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return err
	}
	ok = true
	return nil
}
