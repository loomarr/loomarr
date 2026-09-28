package fillerreview

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/loomarr/loomarr/internal/fillereval"
)

func auditTemporalHumanReviewLeakage(publicRoot string, privateMap TemporalTruthEvidencePrivateMap, selection fillereval.TemporalTruthSelection) error {
	forbidden := make([]string, 0, len(privateMap.Entries)*7)
	forbidden = append(forbidden, privateMap.DraftSHA256, privateMap.DownloadLedgerSHA256, privateMap.PacketsSHA256, privateMap.TranscriptSetSHA256)
	for _, entry := range privateMap.Entries {
		forbidden = append(forbidden, entry.Alias, entry.CaseID, entry.ContentSHA256, entry.SourceLocalFile, entry.SourceSHA256, entry.PacketSHA256, entry.TranscriptSHA256)
	}
	forbidden = append(forbidden, selection.Seed, `"bucket"`, `"riskClass"`, `"rankSha256"`, `"strata"`)
	for _, input := range selection.Inputs {
		forbidden = append(forbidden, input.Name, input.SHA256)
	}
	for _, item := range selection.Cases {
		forbidden = append(forbidden, item.CaseID, item.ContentSHA256, item.RankSHA256)
		forbidden = append(forbidden, item.Strata...)
	}
	return auditTemporalHumanReviewStrings(publicRoot, forbidden)
}

func auditTemporalHumanReviewStrings(root string, forbidden []string) error {
	matcher := newTemporalLeakageMatcher(forbidden)
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		secret, scanErr := matcher.Find(file)
		closeErr := file.Close()
		if scanErr != nil {
			return scanErr
		}
		if closeErr != nil {
			return closeErr
		}
		if secret != "" {
			return fmt.Errorf("public temporal review package leaks coordinator-only value %q in %s", secret, temporalTruthRelative(root, path))
		}
		return nil
	})
}

// temporalLeakageMatcher is an Aho-Corasick matcher. It audits each potentially
// large media byte exactly once while retaining matches that span read chunks.
type temporalLeakageMatcher struct {
	nodes []temporalLeakageNode
}

type temporalLeakageNode struct {
	next  map[byte]int
	fail  int
	match string
}

func newTemporalLeakageMatcher(forbidden []string) temporalLeakageMatcher {
	matcher := temporalLeakageMatcher{nodes: []temporalLeakageNode{{next: map[byte]int{}}}}
	seen := map[string]struct{}{}
	for _, pattern := range forbidden {
		pattern = strings.TrimSpace(pattern)
		if len(pattern) < 4 {
			continue
		}
		if _, duplicate := seen[pattern]; duplicate {
			continue
		}
		seen[pattern] = struct{}{}
		state := 0
		for index := 0; index < len(pattern); index++ {
			next, exists := matcher.nodes[state].next[pattern[index]]
			if !exists {
				next = len(matcher.nodes)
				matcher.nodes[state].next[pattern[index]] = next
				matcher.nodes = append(matcher.nodes, temporalLeakageNode{next: map[byte]int{}})
			}
			state = next
		}
		matcher.nodes[state].match = pattern
	}
	queue := make([]int, 0, len(matcher.nodes))
	for _, state := range matcher.nodes[0].next {
		queue = append(queue, state)
	}
	for len(queue) > 0 {
		state := queue[0]
		queue = queue[1:]
		for value, next := range matcher.nodes[state].next {
			fallback := matcher.nodes[state].fail
			for fallback != 0 {
				if _, exists := matcher.nodes[fallback].next[value]; exists {
					break
				}
				fallback = matcher.nodes[fallback].fail
			}
			if target, exists := matcher.nodes[fallback].next[value]; exists && target != next {
				matcher.nodes[next].fail = target
			}
			if matcher.nodes[next].match == "" {
				matcher.nodes[next].match = matcher.nodes[matcher.nodes[next].fail].match
			}
			queue = append(queue, next)
		}
	}
	return matcher
}

func (matcher temporalLeakageMatcher) Find(reader io.Reader) (string, error) {
	state := 0
	buffer := make([]byte, 64<<10)
	for {
		count, err := reader.Read(buffer)
		for _, value := range buffer[:count] {
			for state != 0 {
				if _, exists := matcher.nodes[state].next[value]; exists {
					break
				}
				state = matcher.nodes[state].fail
			}
			if next, exists := matcher.nodes[state].next[value]; exists {
				state = next
			}
			if matcher.nodes[state].match != "" {
				return matcher.nodes[state].match, nil
			}
		}
		if err == io.EOF {
			return "", nil
		}
		if err != nil {
			return "", err
		}
	}
}
