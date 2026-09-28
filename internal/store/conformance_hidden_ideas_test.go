package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// testHiddenIdeas: a person's hidden channel ideas (#1665) are theirs alone, a repeat hide is a
// no-op, and unhide is the undo.
func testHiddenIdeas(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	mustUpsertPreferenceUser(t, s, "u-ada")
	mustUpsertPreferenceUser(t, s, "u-bo")
	at := time.Unix(1_800_000_000, 0).UTC()

	for _, id := range []string{"genre:comedy", "holiday:halloween", "genre:comedy"} {
		if err := s.HideIdea(ctx, "u-ada", id, at); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.HideIdea(ctx, "u-bo", "decade:1990", at); err != nil {
		t.Fatal(err)
	}
	hidden, err := s.HiddenIdeas(ctx, "u-ada")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(hidden) != "map[genre:comedy:true holiday:halloween:true]" {
		t.Errorf("u-ada hidden = %v, want comedy and halloween only", hidden)
	}

	if err := s.UnhideIdea(ctx, "u-ada", "genre:comedy"); err != nil {
		t.Fatal(err)
	}
	if err := s.UnhideIdea(ctx, "u-ada", "never-hidden"); err != nil {
		t.Errorf("unhiding an idea that isn't hidden: %v, want no error", err)
	}
	hidden, _ = s.HiddenIdeas(ctx, "u-ada")
	if fmt.Sprint(hidden) != "map[holiday:halloween:true]" {
		t.Errorf("after undo u-ada hidden = %v, want halloween only", hidden)
	}

	if hidden, _ := s.HiddenIdeas(ctx, "u-bo"); fmt.Sprint(hidden) != "map[decade:1990:true]" {
		t.Errorf("u-bo hidden = %v, want only their own decade:1990", hidden)
	}
}
