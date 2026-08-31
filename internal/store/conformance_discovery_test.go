package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
)

func testDiscoveryFeedbackMissingChannel(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	for _, action := range []FeedbackAction{FeedbackNever, FeedbackClear} {
		feedback := DiscoveryFeedback{ID: "missing-" + string(action), ActorID: "admin",
			Scope: FeedbackChannel, ScopeID: "missing-channel", Target: provision.Key("movie:tmdb:603"),
			Action: action, CreatedAt: time.Unix(1_700_000_000, 0).UTC()}
		if err := s.AppendDiscoveryFeedback(ctx, feedback); !errors.Is(err, ErrNotFound) {
			t.Errorf("append %s for missing channel = %v, want ErrNotFound", action, err)
		}
	}
	got, err := s.ListDiscoveryFeedback(ctx, FeedbackFilter{Scope: FeedbackChannel, ScopeID: "missing-channel"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("missing-channel feedback = %+v, want no event or clear tombstone", got)
	}
}

func testDiscoveryFeedbackHardDelete(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	channel := sampleChannel("feedback-delete", 31, time.Now().Add(time.Hour))
	mustSaveChannel(t, s, channel)
	otherChannel := sampleChannel("feedback-other", 35, time.Now().Add(time.Hour))
	mustSaveChannel(t, s, otherChannel)
	saved, err := s.GetChannel(ctx, channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	target := provision.Key("movie:tmdb:603")
	for _, feedback := range []DiscoveryFeedback{
		{ID: "delete-channel", ActorID: "admin", Scope: FeedbackChannel, ScopeID: channel.ID,
			Target: target, Action: FeedbackNever, CreatedAt: time.Unix(1_700_000_000, 0).UTC()},
		{ID: "keep-household", ActorID: "admin", Scope: FeedbackHousehold,
			Target: target, Action: FeedbackKeep, CreatedAt: time.Unix(1_700_000_001, 0).UTC()},
		{ID: "keep-other-channel", ActorID: "admin", Scope: FeedbackChannel, ScopeID: otherChannel.ID,
			Target: target, Action: FeedbackSurprise, CreatedAt: time.Unix(1_700_000_002, 0).UTC()},
	} {
		if err := s.AppendDiscoveryFeedback(ctx, feedback); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.DeleteChannel(ctx, channel.ID, saved.Revision); err != nil {
		t.Fatal(err)
	}
	channelFeedback, err := s.ListDiscoveryFeedback(ctx, FeedbackFilter{Scope: FeedbackChannel, ScopeID: channel.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(channelFeedback) != 0 {
		t.Fatalf("hard-deleted channel feedback = %+v, want none", channelFeedback)
	}
	otherFeedback, err := s.ListDiscoveryFeedback(ctx, FeedbackFilter{Scope: FeedbackChannel, ScopeID: otherChannel.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(otherFeedback) != 1 || otherFeedback[0].ID != "keep-other-channel" {
		t.Fatalf("other channel feedback after delete = %+v, want unrelated identity preserved", otherFeedback)
	}
	householdFeedback, err := s.ListDiscoveryFeedback(ctx, FeedbackFilter{Scope: FeedbackHousehold})
	if err != nil {
		t.Fatal(err)
	}
	if len(householdFeedback) != 1 || householdFeedback[0].ID != "keep-household" {
		t.Fatalf("household feedback after channel delete = %+v, want independent event preserved", householdFeedback)
	}
}

func testDiscoveryFeedbackDeleteRace(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	channel := sampleChannel("feedback-race", 33, time.Now().Add(time.Hour))
	mustSaveChannel(t, s, channel)
	saved, err := s.GetChannel(ctx, channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	appendResult := make(chan error, 1)
	deleteResult := make(chan error, 1)
	go func() {
		<-start
		appendResult <- s.AppendDiscoveryFeedback(ctx, DiscoveryFeedback{ID: "racing-append", ActorID: "admin",
			Scope: FeedbackChannel, ScopeID: channel.ID, Target: provision.Key("movie:tmdb:603"),
			Action: FeedbackLess, CreatedAt: time.Unix(1_700_000_000, 0).UTC()})
	}()
	go func() {
		<-start
		deleteResult <- s.DeleteChannel(ctx, channel.ID, saved.Revision)
	}()
	close(start)
	if err := <-deleteResult; err != nil {
		t.Fatalf("racing hard delete: %v", err)
	}
	if err := <-appendResult; err != nil && !errors.Is(err, ErrNotFound) {
		t.Fatalf("racing append = %v, want success-before-delete or ErrNotFound-after-delete", err)
	}
	got, err := s.ListDiscoveryFeedback(ctx, FeedbackFilter{Scope: FeedbackChannel, ScopeID: channel.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("feedback after append/delete race = %+v, want no orphan", got)
	}
}

func testDiscoveryFeedbackDetachedChannel(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	channel := sampleChannel("feedback-detached", 34, time.Now().Add(time.Hour))
	mustSaveChannel(t, s, channel)
	saved, err := s.GetChannel(ctx, channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	saved.Status = schedule.StatusDetached
	if _, err := s.SaveChannel(ctx, saved); err != nil {
		t.Fatal(err)
	}
	feedback := DiscoveryFeedback{ID: "detached-event", ActorID: "admin", Scope: FeedbackChannel,
		ScopeID: channel.ID, Target: provision.Key("movie:tmdb:603"), Action: FeedbackSurprise,
		CreatedAt: time.Unix(1_700_000_000, 0).UTC()}
	if err := s.AppendDiscoveryFeedback(ctx, feedback); err != nil {
		t.Fatalf("append detached-channel feedback: %v", err)
	}
	got, err := s.ListDiscoveryFeedback(ctx, FeedbackFilter{Scope: FeedbackChannel, ScopeID: channel.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != feedback.ID {
		t.Fatalf("detached-channel feedback = %+v, want retained event", got)
	}
}

func testDiscoveryFeedback(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()
	first := DiscoveryFeedback{ID: "f-1", ActorID: "admin", Scope: FeedbackHousehold,
		Target: provision.Key("movie:tmdb:603"), Action: FeedbackKeep, Reason: "family favorite", CreatedAt: base}
	if err := s.AppendDiscoveryFeedback(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendDiscoveryFeedback(ctx, DiscoveryFeedback{ID: "f-2", ActorID: "admin",
		Scope: FeedbackHousehold, Target: first.Target, Action: FeedbackNever, CreatedAt: base.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListDiscoveryFeedback(ctx, FeedbackFilter{Scope: FeedbackHousehold})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "f-2" || got[1].Reason != "family favorite" {
		t.Fatalf("feedback events = %+v, want newest-first append-only history", got)
	}
	mustSaveChannel(t, s, sampleChannel("channel-1", 32, time.Now().Add(time.Hour)))

	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errCh <- s.AppendDiscoveryFeedback(ctx, DiscoveryFeedback{ID: fmt.Sprintf("parallel-%d", i),
				ActorID: "admin", Scope: FeedbackChannel, ScopeID: "channel-1",
				Target: provision.Key(fmt.Sprintf("movie:tmdb:%d", 100+i)), Action: FeedbackLess,
				CreatedAt: base.Add(2 * time.Second)})
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	parallel, err := s.ListDiscoveryFeedback(ctx, FeedbackFilter{Scope: FeedbackChannel, ScopeID: "channel-1"})
	if err != nil || len(parallel) != 8 {
		t.Fatalf("concurrent feedback = %d, %v; want 8 events", len(parallel), err)
	}
}
