package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestDispatchListsEverySubcommand(t *testing.T) {
	var stderr bytes.Buffer
	if code := dispatch([]string{"help"}, &stderr); code != 0 {
		t.Fatalf("help exit = %d", code)
	}
	for _, name := range []string{
		"recommend-cert", "recommend-compare", "recommend-diagnostic",
		"planner-compare", "planner-reference-host", "planner-tool-diagnostic",
	} {
		if _, ok := subcommands[name]; !ok {
			t.Errorf("subcommand %q is not registered", name)
		}
		if !strings.Contains(stderr.String(), "  "+name+" ") {
			t.Errorf("usage does not list %q:\n%s", name, stderr.String())
		}
	}
}

func TestDispatchRejectsMissingAndUnknownSubcommands(t *testing.T) {
	for _, args := range [][]string{nil, {"channel-recommend-cert"}} {
		var stderr bytes.Buffer
		if code := dispatch(args, &stderr); code != 2 || !strings.Contains(stderr.String(), "usage: eval") {
			t.Fatalf("dispatch(%q) = %d; stderr = %s", args, code, stderr.String())
		}
	}
}

func TestDispatchPassesFlagsToTheSubcommand(t *testing.T) {
	var stderr bytes.Buffer
	if code := dispatch([]string{"recommend-compare", "--out"}, &stderr); code != 2 {
		t.Fatalf("exit = %d; stderr = %s", code, stderr.String())
	}
}
