package main

import (
	"strings"
	"testing"
)

func TestSearchCacheIdentityChangesWithQuery(t *testing.T) {
	first, err := archiveSearchURL(defaultBaseURL, "classic_tv_commercials")
	if err != nil {
		t.Fatal(err)
	}
	second := strings.Replace(first, "rows=1000", "rows=500", 1)
	firstKey := sha256Hex([]byte(first))[:16]
	secondKey := sha256Hex([]byte(second))[:16]
	if firstKey == secondKey {
		t.Fatal("different source queries shared a cache identity")
	}
}
