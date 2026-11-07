package releaseverify

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const repositoryScriptTraversalLimit = 32

type repositoryScriptAudit struct {
	root    string
	sources map[string]string
	state   map[string]uint8
	result  map[string]bool
}

func newRepositoryScriptAudit(root string) (*repositoryScriptAudit, error) {
	audit := &repositoryScriptAudit{
		root:    filepath.Clean(root),
		sources: make(map[string]string),
		state:   make(map[string]uint8),
		result:  make(map[string]bool),
	}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			if relative != "." && excludedContainerScanDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if filepath.Ext(path) != ".sh" && !hasShellShebang(source) {
			return nil
		}
		audit.sources[relative] = string(source)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("index repository shell scripts: %w", err)
	}
	return audit, nil
}

func hasShellShebang(source []byte) bool {
	line := string(source)
	if end := strings.IndexByte(line, '\n'); end >= 0 {
		line = line[:end]
	}
	return strings.HasPrefix(line, "#!") && (strings.Contains(line, "/sh") || strings.Contains(line, "bash"))
}

func (audit *repositoryScriptAudit) commandAcquires(command string) bool {
	return audit.commandAcquiresDepth(command, 0)
}

func (audit *repositoryScriptAudit) commandAcquiresDepth(command string, depth int) bool {
	command = stripShellComments(command)
	if depth > repositoryScriptTraversalLimit || scriptTextContainsContainerAcquisition(command) {
		return true
	}
	assignments := make(map[string]string)
	for _, segment := range shellCommandSegments(normalizeShellContinuations(command)) {
		index, updates := leadingShellAssignments(segment)
		if index == len(segment) {
			applyShellAssignments(assignments, updates)
			continue
		}
		executable, resolved := resolveLiteralShellToken(segment[index], assignments)
		if !resolved {
			if relative, script := audit.repositoryScript(segment[index]); script && audit.scriptAcquires(relative, depth+1) {
				return true
			}
			if variableEngineGrammar(segment, index+1) {
				return true
			}
			continue
		}
		var noExecution bool
		index, executable, resolved, noExecution, _ = unwrapShellCommand(segment, index, executable, assignments)
		if !resolved {
			if variableEngineGrammar(segment, index+1) {
				return true
			}
			continue
		}
		if noExecution {
			continue
		}
		if isShellInterpreter(executable) {
			for argument := index + 1; argument < len(segment); argument++ {
				if strings.HasPrefix(segment[argument], "-") {
					continue
				}
				if audit.scriptTokenAcquires(segment[argument], depth+1) {
					return true
				}
				break
			}
			continue
		}
		if audit.scriptTokenAcquires(executable, depth+1) {
			return true
		}
	}
	substitutions, unambiguous := shellCommandSubstitutions(command)
	if !unambiguous {
		return false
	}
	for _, substitution := range substitutions {
		if audit.commandAcquiresDepth(substitution, depth+1) {
			return true
		}
	}
	return false
}

func scriptTextContainsContainerAcquisition(command string) bool {
	for _, segment := range shellCommandSegments(normalizeShellContinuations(command)) {
		var words []string
		for _, token := range segment {
			words = append(words, strings.Fields(token)...)
		}
		for index, word := range words {
			word = strings.Trim(word, `"'{}()<>`)
			if !isContainerEngineExecutable(filepath.Base(word)) {
				continue
			}
			if index+1 < len(words) && words[index+1] == "buildx" {
				continue
			}
			if variableEngineGrammar(words, index+1) {
				return true
			}
		}
	}
	return false
}

func (audit *repositoryScriptAudit) scriptTokenAcquires(token string, depth int) bool {
	relative, ok := audit.repositoryScript(token)
	if !ok {
		return false
	}
	return audit.scriptAcquires(relative, depth)
}

func (audit *repositoryScriptAudit) repositoryScript(token string) (string, bool) {
	token = strings.Trim(token, `"'`)
	if token == "" || strings.Contains(token, "`") {
		return "", false
	}
	if strings.Contains(token, "$") {
		base := filepath.Base(filepath.ToSlash(token))
		if base == "." || base == "" || strings.Contains(base, "$") {
			return "", false
		}
		match := ""
		for candidate := range audit.sources {
			if filepath.Base(candidate) != base {
				continue
			}
			if match != "" {
				return "", false
			}
			match = candidate
		}
		return match, match != ""
	}
	path := token
	if filepath.IsAbs(path) {
		relative, err := filepath.Rel(audit.root, filepath.Clean(path))
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "", false
		}
		path = relative
	}
	path = filepath.ToSlash(filepath.Clean(strings.TrimPrefix(path, "./")))
	_, ok := audit.sources[path]
	return path, ok
}

func (audit *repositoryScriptAudit) scriptAcquires(relative string, depth int) bool {
	if depth > repositoryScriptTraversalLimit {
		return true
	}
	if audit.state[relative] == 1 {
		// A back-edge is not itself acquisition. Any engine grammar in the
		// strongly connected script set is detected before following edges and
		// propagates outward, while this provisional false keeps traversal finite.
		return false
	}
	if audit.state[relative] == 2 {
		return audit.result[relative]
	}
	audit.state[relative] = 1
	audit.result[relative] = audit.commandAcquiresDepth(audit.sources[relative], depth+1)
	audit.state[relative] = 2
	return audit.result[relative]
}
