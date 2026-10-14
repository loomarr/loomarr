// Command release-notes turns GitHub-generated notes into validated, categorized release notes.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/releasenotes"
)

func main() {
	input := flag.String("input", "", "GitHub-generated release-note body")
	output := flag.String("output", "", "destination Markdown file")
	header := flag.String("header", "", "optional curated Markdown header")
	model := flag.String("model", envOr("RELEASE_NOTES_MODEL", releasenotes.DefaultModel), "OpenRouter model")
	flag.Parse()
	if err := run(*input, *output, *header, *model); err != nil {
		fmt.Fprintf(os.Stderr, "release-notes: %v\n", err)
		os.Exit(1)
	}
}

func run(input, output, header, model string) error {
	if input == "" || output == "" {
		return errors.New("--input and --output are required")
	}
	native, err := os.ReadFile(input)
	if err != nil {
		return fmt.Errorf("read generated notes: %w", err)
	}
	doc, err := releasenotes.Parse(string(native))
	if err != nil {
		return err
	}
	var classification releasenotes.Classification
	if len(doc.Changes) > 0 {
		classifier := releasenotes.OpenRouter{APIKey: os.Getenv("OPENROUTER_RELEASE_API_KEY"), Model: model}
		var classifyErr error
		for attempt := 1; attempt <= 3; attempt++ {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			classification, classifyErr = classifier.Classify(ctx, doc)
			cancel()
			if classifyErr == nil {
				break
			}
			if attempt < 3 {
				time.Sleep(time.Duration(attempt) * time.Second)
			}
		}
		if classifyErr != nil {
			return fmt.Errorf("categorize after 3 attempts: %w", classifyErr)
		}
	}
	formatted, err := releasenotes.Render(doc, classification)
	if err != nil {
		return err
	}
	if header != "" {
		curated, readErr := os.ReadFile(header)
		if readErr != nil {
			return fmt.Errorf("read curated header: %w", readErr)
		}
		formatted = strings.TrimSpace(string(curated)) + "\n\n" + formatted
	}
	if err := os.WriteFile(output, []byte(formatted), 0o600); err != nil {
		return fmt.Errorf("write release notes: %w", err)
	}
	return nil
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
