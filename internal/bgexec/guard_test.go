package bgexec

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// fillerOwned reports whether a package directory (relative to the repo root) is filler-owned:
// the filler pipeline itself, every fillerXXX package and command, mediatools and clipfetch, whose ffmpeg
// wrappers the pipeline calls and whose yt-dlp spawns ffmpeg.
func fillerOwned(dir string) bool {
	dir = filepath.ToSlash(dir)
	switch {
	case dir == "internal/mediatools", dir == "internal/clipfetch":
		return true
	case strings.HasPrefix(dir, "internal/filler"):
		return true
	case strings.HasPrefix(dir, "cmd/filler"):
		return true
	}
	return false
}

// #1512 G5: every filler-owned media process must start through this package so it is niced and
// (for ffmpeg) thread-capped. This guard fails when a filler-owned, non-test file starts a process
// any other way — a new call site cannot quietly reintroduce an uncapped 350% ffmpeg.
func TestFillerOwnedPackagesStartProcessesOnlyThroughBgexec(t *testing.T) {
	root := filepath.Join("..", "..")
	var scanned int
	for _, top := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			if d.IsDir() {
				if strings.HasPrefix(d.Name(), ".") || d.Name() == "testdata" || d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || !fillerOwned(filepath.Dir(rel)) {
				return nil
			}
			scanned++
			checkFile(t, path, rel)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if scanned < 100 {
		t.Fatalf("scanned only %d filler-owned files; the guard is not seeing the tree", scanned)
	}
}

func checkFile(t *testing.T, path, rel string) {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Errorf("parse %s: %v", rel, err)
		return
	}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		switch {
		case pkg.Name == "exec" && (sel.Sel.Name == "Command" || sel.Sel.Name == "CommandContext"):
			t.Errorf("%s starts a process with exec.%s; use bgexec.FFmpeg / bgexec.Tool / bgexec.Whisper", rel, sel.Sel.Name)
		case pkg.Name == "proctree" && sel.Sel.Name == "Start":
			t.Errorf("%s calls proctree.Start directly; use bgexec so the process is niced", rel)
		case pkg.Name == "bgexec" && (sel.Sel.Name == "Tool" || sel.Sel.Name == "Whisper") && passesInputFlag(call):
			t.Errorf("%s runs ffmpeg through bgexec.%s, which does not cap threads; use bgexec.FFmpeg", rel, sel.Sel.Name)
		}
		return true
	})
}

// passesInputFlag spots a literal "-i" argument: only ffmpeg takes one, and ffmpeg belongs on
// bgexec.FFmpeg so its threads are capped.
func passesInputFlag(call *ast.CallExpr) bool {
	for _, arg := range call.Args {
		if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING && lit.Value == `"-i"` {
			return true
		}
	}
	return false
}
