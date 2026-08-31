package testkit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// Executable writes a shared system-boundary test double for code that invokes a required local
// tool. Keep these doubles in testkit rather than growing one private shell mock per caller.
func Executable(t *testing.T, name, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write executable %s: %v", name, err)
	}
	return path
}

// CopyingMediaExecutable builds a portable local-tool double that copies the argument following
// -i to the final argument, appends suffix, and emits ffmpeg's terminal progress marker.
func CopyingMediaExecutable(t *testing.T, name, suffix string, requiredArguments ...string) string {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "main.go")
	quoted := make([]string, 0, len(requiredArguments))
	for _, required := range requiredArguments {
		quoted = append(quoted, strconv.Quote(required))
	}
	program := fmt.Sprintf(`package main
import (
	"fmt"
	"os"
	"strings"
)
func main() {
	args := os.Args[1:]
	joined := strings.Join(args, " ")
	for _, required := range []string{%s} {
		if !strings.Contains(joined, required) { os.Exit(5) }
	}
	input := ""
	for i := 1; i < len(args); i++ {
		if args[i-1] == "-i" { input = args[i] }
	}
	if input == "" || len(args) == 0 { os.Exit(2) }
	data, err := os.ReadFile(input)
	if err != nil { os.Exit(3) }
	data = append(data, []byte(%q)...)
	if err := os.WriteFile(args[len(args)-1], data, 0600); err != nil { os.Exit(4) }
	fmt.Print("progress=end\n")
}
`, strings.Join(quoted, ","), suffix)
	if err := os.WriteFile(source, []byte(program), 0o600); err != nil {
		t.Fatalf("write portable executable source: %v", err)
	}
	executable := filepath.Join(dir, name)
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", executable, source)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build portable executable %s: %v: %s", name, err, output)
	}
	return executable
}
