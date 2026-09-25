// Command graphify-sync adds GitHub Actions structure and Loomarr's source
// freshness stamp to the repository knowledge graph.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	graphifyVersion = "graphifyy[sql]==0.9.64"
	metadataKey     = "loomarr_graphify"
	provenanceKey   = "loomarr_origin"
	origin          = "loomarr-workflow"
	schemaVersion   = 1
)

var codeExtensions = map[string]struct{}{
	".astro": {}, ".bash": {}, ".c": {}, ".cc": {}, ".cjs": {}, ".cl": {},
	".cls": {}, ".cpp": {}, ".cs": {}, ".cshtml": {}, ".csproj": {}, ".cts": {},
	".cu": {}, ".cuh": {}, ".cxx": {}, ".dart": {}, ".dfm": {}, ".dm": {},
	".dme": {}, ".dmf": {}, ".dmi": {}, ".dmm": {}, ".dpk": {}, ".dpr": {},
	".ejs": {}, ".ets": {}, ".ex": {}, ".exs": {}, ".f": {}, ".f03": {},
	".f08": {}, ".f90": {}, ".f95": {}, ".fsproj": {}, ".go": {}, ".gradle": {},
	".groovy": {}, ".h": {}, ".hcl": {}, ".hh": {}, ".hpp": {}, ".hxx": {},
	".inc": {}, ".java": {}, ".jl": {}, ".js": {}, ".json": {}, ".jsx": {},
	".kt": {}, ".kts": {}, ".lfm": {}, ".lisp": {}, ".lpk": {}, ".lpr": {},
	".lsp": {}, ".lua": {}, ".luau": {}, ".m": {}, ".metal": {}, ".mjs": {},
	".ml": {}, ".mli": {}, ".mm": {}, ".mts": {}, ".pas": {}, ".php": {},
	".pp": {}, ".ps1": {}, ".psd1": {}, ".psm1": {}, ".py": {}, ".r": {},
	".rake": {}, ".razor": {}, ".rb": {}, ".resource": {}, ".robot": {}, ".rs": {},
	".scala": {}, ".sh": {}, ".sln": {}, ".slnx": {}, ".sql": {}, ".svelte": {},
	".sv": {}, ".svh": {}, ".swift": {}, ".tf": {}, ".tfvars": {}, ".toc": {},
	".trigger": {}, ".ts": {}, ".tsx": {}, ".v": {}, ".vbproj": {}, ".vue": {},
	".xaml": {}, ".zig": {},
}

var packageManifestNames = map[string]struct{}{
	"apm.yaml": {}, "apm.yml": {}, "cargo.toml": {}, "go.mod": {},
	"pom.xml": {}, "pyproject.toml": {},
}

type graphMetadata struct {
	Schema        int    `json:"schema"`
	Graphify      string `json:"graphify"`
	InputsSHA256  string `json:"inputs_sha256"`
	SourceFiles   int    `json:"source_files"`
	WorkflowFiles int    `json:"workflow_files"`
}

type projection struct {
	Nodes []map[string]any
	Links []map[string]any
}

func main() {
	root := flag.String("root", ".", "repository root")
	graphPath := flag.String("graph", "graphify-out/graph.json", "graph JSON path relative to root")
	flag.Parse()
	if flag.NArg() != 1 {
		fatalf("usage: graphify-sync [flags] inject|stamp|verify")
	}
	absRoot, err := filepath.Abs(*root)
	if err != nil {
		fatalf("resolve root: %v", err)
	}
	absGraph := *graphPath
	if !filepath.IsAbs(absGraph) {
		absGraph = filepath.Join(absRoot, absGraph)
	}

	switch flag.Arg(0) {
	case "inject":
		if err := inject(absRoot, absGraph); err != nil {
			fatalf("inject workflows: %v", err)
		}
	case "stamp":
		if err := stamp(absRoot, absGraph); err != nil {
			fatalf("stamp graph: %v", err)
		}
	case "verify":
		if err := verify(absRoot, absGraph); err != nil {
			fatalf("graph is stale: %v\nrun `make graphify` and commit graphify-out/", err)
		}
	default:
		fatalf("unknown operation %q", flag.Arg(0))
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "graphify-sync: "+format+"\n", args...)
	os.Exit(1)
}

func inject(root, graphPath string) error {
	doc, err := readGraph(graphPath)
	if err != nil {
		return err
	}
	want, err := buildWorkflowProjection(root)
	if err != nil {
		return err
	}
	removeProjection(doc)
	appendProjection(doc, want)
	return writeGraph(graphPath, doc)
}

func stamp(root, graphPath string) error {
	doc, err := readGraph(graphPath)
	if err != nil {
		return err
	}
	metadata, err := currentMetadata(root)
	if err != nil {
		return err
	}
	graph, _ := doc["graph"].(map[string]any)
	if graph == nil {
		graph = make(map[string]any)
		doc["graph"] = graph
	}
	graph[metadataKey] = metadata
	return writeGraph(graphPath, doc)
}

func verify(root, graphPath string) error {
	doc, err := readGraph(graphPath)
	if err != nil {
		return err
	}
	wantMetadata, err := currentMetadata(root)
	if err != nil {
		return err
	}
	graph, _ := doc["graph"].(map[string]any)
	gotMetadata, ok := graph[metadataKey]
	if !ok {
		return errors.New("missing Loomarr freshness metadata")
	}
	if !jsonEqual(gotMetadata, wantMetadata) {
		got, _ := json.Marshal(gotMetadata)
		want, _ := json.Marshal(wantMetadata)
		return fmt.Errorf("input fingerprint differs: got %s, want %s", got, want)
	}

	wantProjection, err := buildWorkflowProjection(root)
	if err != nil {
		return err
	}
	gotProjection := selectProjection(doc)
	if err := compareProjection(gotProjection, wantProjection); err != nil {
		return fmt.Errorf("GitHub Actions projection differs: %w", err)
	}
	fmt.Printf("graphify-verify: current (%d source files, %d workflows)\n", wantMetadata.SourceFiles, wantMetadata.WorkflowFiles)
	return nil
}

func readGraph(path string) (map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	decoder := json.NewDecoder(f)
	decoder.UseNumber()
	var doc map[string]any
	if err := decoder.Decode(&doc); err != nil {
		return nil, err
	}
	if _, ok := doc["nodes"].([]any); !ok {
		return nil, errors.New("graph has no nodes array")
	}
	if _, err := graphEdges(doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func graphEdges(doc map[string]any) (string, error) {
	if _, ok := doc["links"].([]any); ok {
		return "links", nil
	}
	if _, ok := doc["edges"].([]any); ok {
		return "edges", nil
	}
	return "", errors.New("graph has neither a links nor edges array")
}

func writeGraph(path string, doc map[string]any) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".graph.json-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	encoder := json.NewEncoder(tmp)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(doc); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func removeProjection(doc map[string]any) {
	removed := make(map[string]struct{})
	nodes := doc["nodes"].([]any)
	keptNodes := nodes[:0]
	for _, raw := range nodes {
		node, _ := raw.(map[string]any)
		if node[provenanceKey] == origin || node["_origin"] == origin {
			if id, ok := node["id"].(string); ok {
				removed[id] = struct{}{}
			}
			continue
		}
		keptNodes = append(keptNodes, raw)
	}
	doc["nodes"] = keptNodes

	edgesKey, _ := graphEdges(doc)
	links := doc[edgesKey].([]any)
	keptLinks := links[:0]
	for _, raw := range links {
		link, _ := raw.(map[string]any)
		_, sourceRemoved := removed[stringValue(link["source"])]
		_, targetRemoved := removed[stringValue(link["target"])]
		if link[provenanceKey] == origin || link["_origin"] == origin || sourceRemoved || targetRemoved {
			continue
		}
		keptLinks = append(keptLinks, raw)
	}
	doc[edgesKey] = keptLinks
}

func appendProjection(doc map[string]any, p projection) {
	nodes := doc["nodes"].([]any)
	for _, node := range p.Nodes {
		nodes = append(nodes, node)
	}
	edgesKey, _ := graphEdges(doc)
	links := doc[edgesKey].([]any)
	for _, link := range p.Links {
		links = append(links, link)
	}
	doc["nodes"] = nodes
	doc[edgesKey] = links
}

func selectProjection(doc map[string]any) projection {
	var got projection
	for _, raw := range doc["nodes"].([]any) {
		node, _ := raw.(map[string]any)
		if node[provenanceKey] == origin {
			got.Nodes = append(got.Nodes, node)
		}
	}
	edgesKey, _ := graphEdges(doc)
	for _, raw := range doc[edgesKey].([]any) {
		link, _ := raw.(map[string]any)
		if link[provenanceKey] == origin {
			got.Links = append(got.Links, link)
		}
	}
	return got
}

func compareProjection(got, want projection) error {
	nodeFields := []string{"id", "label", "_origin", provenanceKey, "external", "file_type", "source_file", "source_location", "source_sha256", "type"}
	linkFields := []string{"source", "target", "relation", "_origin", provenanceKey, "confidence", "confidence_score", "context", "source_file", "source_location", "weight"}
	gotNodes := canonicalRecords(got.Nodes, nodeFields)
	wantNodes := canonicalRecords(want.Nodes, nodeFields)
	if !bytes.Equal(gotNodes, wantNodes) {
		return fmt.Errorf("nodes differ (%d committed, %d generated)", len(got.Nodes), len(want.Nodes))
	}
	gotLinks := canonicalRecords(got.Links, linkFields)
	wantLinks := canonicalRecords(want.Links, linkFields)
	if !bytes.Equal(gotLinks, wantLinks) {
		return fmt.Errorf("links differ (%d committed, %d generated)", len(got.Links), len(want.Links))
	}
	return nil
}

func canonicalRecords(records []map[string]any, fields []string) []byte {
	rows := make([]string, 0, len(records))
	for _, record := range records {
		selected := make(map[string]any)
		for _, field := range fields {
			if value, ok := record[field]; ok {
				selected[field] = value
			}
		}
		// readGraph preserves JSON numbers with UseNumber while a freshly built
		// projection contains Go floats. Normalize both representations before
		// comparing so 1 and 1.0 do not make an unchanged graph appear stale.
		encoded := canonicalJSON(selected)
		rows = append(rows, string(encoded))
	}
	sort.Strings(rows)
	return []byte(strings.Join(rows, "\n"))
}

func currentMetadata(root string) (graphMetadata, error) {
	files, err := repositoryFiles(root)
	if err != nil {
		return graphMetadata{}, err
	}
	ignore, err := readGraphifyIgnore(filepath.Join(root, ".graphifyignore"))
	if err != nil {
		return graphMetadata{}, err
	}
	inputs := make([]string, 0, len(files))
	workflowCount := 0
	for _, path := range files {
		path = filepath.ToSlash(path)
		workflow := isWorkflow(path)
		if workflow {
			workflowCount++
		}
		if path == ".graphifyignore" || workflow || (isCodeInput(root, path) && !ignore.matches(path)) {
			inputs = append(inputs, path)
		}
	}
	sort.Strings(inputs)
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "loomarr-graphify-inputs-v%d\x00%s\x00", schemaVersion, graphifyVersion)
	for _, path := range inputs {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return graphMetadata{}, fmt.Errorf("read input %s: %w", path, err)
		}
		sum := sha256.Sum256(data)
		h.Write([]byte(path))
		h.Write([]byte{0})
		h.Write(sum[:])
	}
	return graphMetadata{
		Schema:        schemaVersion,
		Graphify:      graphifyVersion,
		InputsSHA256:  hex.EncodeToString(h.Sum(nil)),
		SourceFiles:   len(inputs),
		WorkflowFiles: workflowCount,
	}, nil
}

func repositoryFiles(root string) ([]string, error) {
	cmd := exec.Command("git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list repository files: %w", err)
	}
	parts := bytes.Split(out, []byte{0})
	files := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) != 0 {
			files = append(files, string(part))
		}
	}
	return files, nil
}

func isCodeInput(root, path string) bool {
	base := strings.ToLower(filepath.Base(path))
	if _, ok := packageManifestNames[base]; ok {
		return true
	}
	ext := strings.ToLower(filepath.Ext(path))
	if _, ok := codeExtensions[ext]; ok {
		return true
	}
	if ext != "" {
		return false
	}
	f, err := os.Open(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	line, _ := bufio.NewReader(io.LimitReader(f, 512)).ReadString('\n')
	if !strings.HasPrefix(line, "#!") {
		return false
	}
	for _, interpreter := range []string{"python", "ruby", "perl", "node", "bash", "sh", "dash", "zsh", "fish", "ksh", "tcsh", "lua", "php", "julia", "rscript"} {
		if strings.Contains(strings.ToLower(line), interpreter) {
			return true
		}
	}
	return false
}

func isWorkflow(path string) bool {
	if !strings.HasPrefix(path, ".github/workflows/") {
		return false
	}
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yml" || ext == ".yaml"
}

type ignoreRules []string

func readGraphifyIgnore(path string) (ignoreRules, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rules ignoreRules
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.ContainsAny(strings.TrimPrefix(line, "**/"), "*?[") {
			return nil, fmt.Errorf("unsupported .graphifyignore pattern %q", line)
		}
		rules = append(rules, filepath.ToSlash(line))
	}
	return rules, nil
}

func (rules ignoreRules) matches(path string) bool {
	for _, rule := range rules {
		anywhere := strings.HasPrefix(rule, "**/")
		rule = strings.TrimPrefix(rule, "**/")
		if strings.HasSuffix(rule, "/") {
			dir := strings.TrimSuffix(rule, "/")
			if strings.HasPrefix(path, rule) || (anywhere && hasPathComponent(path, dir)) {
				return true
			}
			continue
		}
		if path == rule || (anywhere && (strings.HasSuffix(path, "/"+rule) || path == rule)) {
			return true
		}
	}
	return false
}

func hasPathComponent(path, component string) bool {
	for _, part := range strings.Split(path, "/") {
		if part == component {
			return true
		}
	}
	return false
}

func buildWorkflowProjection(root string) (projection, error) {
	paths, err := filepath.Glob(filepath.Join(root, ".github", "workflows", "*.y*ml"))
	if err != nil {
		return projection{}, err
	}
	sort.Strings(paths)
	nodes := make(map[string]map[string]any)
	links := make(map[string]map[string]any)
	addNode := func(node map[string]any) {
		id := node["id"].(string)
		current, exists := nodes[id]
		_, currentIsComplete := current["source_sha256"]
		_, candidateIsComplete := node["source_sha256"]
		if !exists || candidateIsComplete || !currentIsComplete {
			nodes[id] = node
		}
	}
	addLink := func(link map[string]any) {
		key := fmt.Sprintf("%s\x00%s\x00%s", link["source"], link["target"], link["relation"])
		links[key] = link
	}

	for _, path := range paths {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return projection{}, err
		}
		rel = filepath.ToSlash(rel)
		data, err := os.ReadFile(path)
		if err != nil {
			return projection{}, err
		}
		var document yaml.Node
		if err := yaml.Unmarshal(data, &document); err != nil {
			return projection{}, fmt.Errorf("parse %s: %w", rel, err)
		}
		if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
			return projection{}, fmt.Errorf("%s is not a YAML mapping", rel)
		}
		rootNode := document.Content[0]
		workflowID := stableID("workflow", rel)
		workflowName := scalar(rootNode, "name")
		if workflowName == "" {
			workflowName = strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
		}
		sum := sha256.Sum256(data)
		addNode(graphNode(workflowID, workflowName, "workflow", rel, rootNode.Line, map[string]any{
			"source_sha256": hex.EncodeToString(sum[:]),
		}))

		if triggerNode, ok := mappingValue(rootNode, "on"); ok {
			for _, trigger := range yamlKeys(triggerNode) {
				triggerID := stableID("workflow-trigger", trigger)
				addNode(graphNode(triggerID, trigger, "workflow_trigger", "", 0, map[string]any{"external": true}))
				addLink(graphLink(workflowID, triggerID, "triggers_on", rel, triggerNode.Line, "workflow trigger"))
			}
		}

		jobsNode, ok := mappingValue(rootNode, "jobs")
		if !ok || jobsNode.Kind != yaml.MappingNode {
			continue
		}
		jobIDs := make(map[string]string)
		for i := 0; i+1 < len(jobsNode.Content); i += 2 {
			jobKey, jobNode := jobsNode.Content[i], jobsNode.Content[i+1]
			jobIDs[jobKey.Value] = stableID("workflow-job", rel, jobKey.Value)
			label := scalar(jobNode, "name")
			if label == "" {
				label = jobKey.Value
			}
			addNode(graphNode(jobIDs[jobKey.Value], label, "workflow_job", rel, jobKey.Line, map[string]any{"workflow_job_id": jobKey.Value}))
			addLink(graphLink(workflowID, jobIDs[jobKey.Value], "contains", rel, jobKey.Line, "workflow job"))
		}
		for i := 0; i+1 < len(jobsNode.Content); i += 2 {
			jobKey, jobNode := jobsNode.Content[i], jobsNode.Content[i+1]
			jobID := jobIDs[jobKey.Value]
			if needsNode, ok := mappingValue(jobNode, "needs"); ok {
				for _, need := range yamlValues(needsNode) {
					if target, exists := jobIDs[need]; exists {
						addLink(graphLink(jobID, target, "needs", rel, needsNode.Line, "job dependency"))
					}
				}
			}
			if uses := scalarNode(jobNode, "uses"); uses != nil {
				targetID, targetNode := usesTarget(rel, uses.Value)
				addNode(targetNode)
				addLink(graphLink(jobID, targetID, "uses", rel, uses.Line, "reusable workflow"))
			}
			steps, ok := mappingValue(jobNode, "steps")
			if !ok || steps.Kind != yaml.SequenceNode {
				continue
			}
			for index, step := range steps.Content {
				stepID := stableID("workflow-step", rel, jobKey.Value, fmt.Sprint(index))
				label := scalar(step, "name")
				uses := scalarNode(step, "uses")
				run := scalarNode(step, "run")
				if label == "" && uses != nil {
					label = uses.Value
				}
				if label == "" && run != nil {
					label = firstLine(run.Value)
				}
				if label == "" {
					label = fmt.Sprintf("step %d", index+1)
				}
				addNode(graphNode(stepID, label, "workflow_step", rel, step.Line, nil))
				addLink(graphLink(jobID, stepID, "contains", rel, step.Line, "workflow step"))
				if uses != nil {
					targetID, targetNode := usesTarget(rel, uses.Value)
					addNode(targetNode)
					addLink(graphLink(stepID, targetID, "uses", rel, uses.Line, "workflow action"))
				}
			}
		}
	}

	result := projection{Nodes: make([]map[string]any, 0, len(nodes)), Links: make([]map[string]any, 0, len(links))}
	for _, node := range nodes {
		result.Nodes = append(result.Nodes, node)
	}
	for _, link := range links {
		result.Links = append(result.Links, link)
	}
	sort.Slice(result.Nodes, func(i, j int) bool { return result.Nodes[i]["id"].(string) < result.Nodes[j]["id"].(string) })
	sort.Slice(result.Links, func(i, j int) bool {
		left := fmt.Sprintf("%s\x00%s\x00%s", result.Links[i]["source"], result.Links[i]["target"], result.Links[i]["relation"])
		right := fmt.Sprintf("%s\x00%s\x00%s", result.Links[j]["source"], result.Links[j]["target"], result.Links[j]["relation"])
		return left < right
	})
	return result, nil
}

func graphNode(id, label, kind, source string, line int, extra map[string]any) map[string]any {
	node := map[string]any{
		"id": id, "label": label, "_origin": "ast", provenanceKey: origin, "file_type": "code",
		"source_file": source, "type": kind,
	}
	if line > 0 {
		node["source_location"] = fmt.Sprintf("L%d", line)
	}
	for key, value := range extra {
		node[key] = value
	}
	return node
}

func graphLink(source, target, relation, file string, line int, context string) map[string]any {
	link := map[string]any{
		"source": source, "target": target, "relation": relation, "_origin": "ast", provenanceKey: origin,
		"confidence": "EXTRACTED", "confidence_score": 1.0, "context": context,
		"source_file": file, "weight": 1.0,
	}
	if line > 0 {
		link["source_location"] = fmt.Sprintf("L%d", line)
	}
	return link
}

func usesTarget(sourceFile, value string) (string, map[string]any) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "./.github/workflows/") {
		path := strings.TrimPrefix(value, "./")
		id := stableID("workflow", path)
		return id, graphNode(id, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), "workflow", path, 1, nil)
	}
	id := stableID("workflow-action", value)
	return id, graphNode(id, value, "workflow_action", "", 0, map[string]any{"external": true})
}

func stableID(kind string, parts ...string) string {
	h := sha256.New()
	h.Write([]byte(kind))
	for _, part := range parts {
		h.Write([]byte{0})
		h.Write([]byte(part))
	}
	return "loomarr_" + strings.ReplaceAll(kind, "-", "_") + "_" + hex.EncodeToString(h.Sum(nil))[:24]
}

func mappingValue(mapping *yaml.Node, key string) (*yaml.Node, bool) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil, false
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1], true
		}
	}
	return nil, false
}

func scalar(mapping *yaml.Node, key string) string {
	node := scalarNode(mapping, key)
	if node == nil {
		return ""
	}
	return strings.TrimSpace(node.Value)
}

func scalarNode(mapping *yaml.Node, key string) *yaml.Node {
	node, ok := mappingValue(mapping, key)
	if !ok || node.Kind != yaml.ScalarNode {
		return nil
	}
	return node
}

func yamlKeys(node *yaml.Node) []string {
	switch node.Kind {
	case yaml.MappingNode:
		values := make([]string, 0, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			values = append(values, node.Content[i].Value)
		}
		return values
	case yaml.SequenceNode:
		return yamlValues(node)
	case yaml.ScalarNode:
		if node.Value != "" {
			return []string{node.Value}
		}
	}
	return nil
}

func yamlValues(node *yaml.Node) []string {
	if node.Kind == yaml.ScalarNode {
		return []string{node.Value}
	}
	if node.Kind != yaml.SequenceNode {
		return nil
	}
	values := make([]string, 0, len(node.Content))
	for _, child := range node.Content {
		if child.Kind == yaml.ScalarNode {
			values = append(values, child.Value)
		}
	}
	return values
}

func firstLine(value string) string {
	line := strings.TrimSpace(strings.Split(value, "\n")[0])
	if len(line) > 80 {
		return line[:77] + "..."
	}
	return line
}

func stringValue(value any) string {
	result, _ := value.(string)
	return result
}

func jsonEqual(left, right any) bool {
	a := canonicalJSON(left)
	b := canonicalJSON(right)
	return bytes.Equal(a, b)
}

func canonicalJSON(value any) []byte {
	encoded, _ := json.Marshal(value)
	var normalized any
	_ = json.Unmarshal(encoded, &normalized)
	encoded, _ = json.Marshal(normalized)
	return encoded
}
