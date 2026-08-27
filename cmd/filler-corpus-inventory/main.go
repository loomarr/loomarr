// Command filler-corpus-inventory deterministically combines strict schema-v3
// source captures into one mixed-authority certification inventory.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/loomarr/loomarr/internal/fillercorpus"
)

type paths []string

func (p *paths) String() string { return fmt.Sprint([]string(*p)) }
func (p *paths) Set(value string) error {
	if value == "" {
		return fmt.Errorf("inventory path is empty")
	}
	*p = append(*p, value)
	return nil
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-corpus-inventory", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var inputs paths
	flags.Var(&inputs, "inventory", "strict schema-v3 source inventory; repeat for every capture file")
	output := flags.String("out", "", "combined mixed-authority inventory JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if len(inputs) == 0 || *output == "" {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-inventory: at least one --inventory and --out are required")
		return 2
	}
	values := make([]fillercorpus.Inventory, 0, len(inputs))
	for _, path := range inputs {
		raw, err := os.ReadFile(path)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "filler-corpus-inventory: read %s: %v\n", path, err)
			return 1
		}
		value, err := fillercorpus.DecodeInventoryBytes(raw)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "filler-corpus-inventory: %s: %v\n", path, err)
			return 1
		}
		values = append(values, value)
	}
	merged, err := fillercorpus.MergeInventories(values...)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-inventory:", err)
		return 1
	}
	if err := writeJSON(*output, merged); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-inventory: write:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-corpus-inventory: froze %d cases from %d captures\n", len(merged.Cases), len(merged.Captures))
	return 0
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".filler-corpus-inventory-*")
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
	if _, err := temp.Write(append(data, '\n')); err != nil {
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
