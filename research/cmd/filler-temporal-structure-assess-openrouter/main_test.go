package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRequiresPaidStructureAuthority(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "positive case/request/cost ceilings") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestAliasListRejectsEmpty(t *testing.T) {
	var aliases aliasList
	if err := aliases.Set(" "); err == nil {
		t.Fatal("empty alias accepted")
	}
	if err := aliases.Set("case-a"); err != nil || aliases.String() != "case-a" {
		t.Fatalf("aliases=%v err=%v", aliases, err)
	}
}
