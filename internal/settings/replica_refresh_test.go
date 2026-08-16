package settings

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/testkit"
)

func cloneSnapshot(in Snapshot) Snapshot {
	out := Snapshot{
		Values:       make(map[string]string, len(in.Values)),
		EnvOverrides: make(map[string]bool, len(in.EnvOverrides)),
	}
	for key, value := range in.Values {
		out.Values[key] = value
	}
	for key, value := range in.EnvOverrides {
		out.EnvOverrides[key] = value
	}
	return out
}

func TestReplicaRefresh_OneReadPublishesWholeSnapshotAndCancels(t *testing.T) {
	loader := testkit.NewSnapshotLoader(Snapshot{Values: map[string]string{
		"library.flavor": "emby",
		"library.url":    "http://old:8096",
		"library.token":  "old-token",
	}}, cloneSnapshot)
	svc, err := New(context.Background(), NewRegistry(), loader, nil)
	if err != nil {
		t.Fatal(err)
	}
	loader.Set(Snapshot{
		Values: map[string]string{
			"library.flavor": "jellyfin",
			"library.url":    "http://new:8096",
			"library.token":  "new-token",
		},
		EnvOverrides: map[string]bool{"library.token": true},
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time)
	refreshed := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		svc.refreshOn(ctx, ticks, func() { refreshed <- struct{}{} })
		close(done)
	}()
	ticks <- time.Unix(1, 0)
	select {
	case <-refreshed:
	case <-time.After(time.Second):
		t.Fatal("refresh did not complete")
	}

	conn := svc.LibraryConnection()
	if conn.Flavor != "jellyfin" || conn.BaseURL != "http://new:8096" || conn.Token != "new-token" {
		t.Fatalf("connection = %+v, want complete new generation", conn)
	}
	if !svc.Resolve("library.token").EnvOverride {
		t.Fatal("environment ownership did not publish with the value snapshot")
	}
	if got := loader.Reads(); got != 2 {
		t.Fatalf("loader reads = %d, want one boot read plus one refresh read", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("refresher did not stop after cancellation")
	}
}

func TestReplicaRefresh_FailureKeepsPriorSnapshotAndRetries(t *testing.T) {
	loader := testkit.NewSnapshotLoader(Snapshot{Values: map[string]string{
		"library.flavor": "emby", "library.url": "http://old:8096", "library.token": "old-token",
	}}, cloneSnapshot)
	svc, err := New(context.Background(), NewRegistry(), loader, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := make(chan time.Time)
	refreshed := make(chan struct{}, 1)
	go svc.refreshOn(ctx, ticks, func() { refreshed <- struct{}{} })

	loader.Set(Snapshot{}, errors.New("database unavailable"))
	ticks <- time.Unix(1, 0)
	if got := svc.LibraryConnection().BaseURL; got != "http://old:8096" {
		t.Fatalf("failed refresh published %q, want prior snapshot", got)
	}

	loader.Set(Snapshot{Values: map[string]string{
		"library.flavor": "jellyfin", "library.url": "http://new:8096", "library.token": "new-token",
	}}, nil)
	ticks <- time.Unix(2, 0)
	select {
	case <-refreshed:
	case <-time.After(time.Second):
		t.Fatal("successful retry did not complete")
	}
	if got := svc.LibraryConnection().BaseURL; got != "http://new:8096" {
		t.Fatalf("retry published %q, want new snapshot", got)
	}
}

func TestReplicaRefresh_ResolveManyNeverMixesGenerations(t *testing.T) {
	old := Snapshot{Values: map[string]string{
		"library.flavor": "emby", "library.url": "http://old:8096", "library.token": "old-token",
	}}
	newer := Snapshot{Values: map[string]string{
		"library.flavor": "jellyfin", "library.url": "http://new:8096", "library.token": "new-token",
	}}
	loader := testkit.NewSnapshotLoader(old, cloneSnapshot)
	svc, err := New(context.Background(), NewRegistry(), loader, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		svc.refreshOn(ctx, ticks, nil)
		close(done)
	}()
	for i := range 500 {
		if i%2 == 0 {
			loader.Set(newer, nil)
		} else {
			loader.Set(old, nil)
		}
		ticks <- time.Unix(int64(i+1), 0)
		conn := svc.LibraryConnection()
		isOld := conn.Flavor == "emby" && conn.BaseURL == "http://old:8096" && conn.Token == "old-token"
		isNew := conn.Flavor == "jellyfin" && conn.BaseURL == "http://new:8096" && conn.Token == "new-token"
		if !isOld && !isNew {
			t.Fatalf("mixed connection generation: %+v", conn)
		}
	}
	cancel()
	<-done
}

func TestReplicaRefresh_OwnershipChangeNotifiesWatchers(t *testing.T) {
	snapshot := Snapshot{Values: map[string]string{"library.url": "http://stored:8096"}}
	loader := testkit.NewSnapshotLoader(snapshot, cloneSnapshot)
	svc, err := New(context.Background(), NewRegistry(), loader, nil)
	if err != nil {
		t.Fatal(err)
	}
	watch := svc.Watch("library.url")
	snapshot.EnvOverrides = map[string]bool{"library.url": true}
	loader.Set(snapshot, nil)
	if err := svc.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case change := <-watch:
		if change.Key != "library.url" {
			t.Fatalf("change key = %q, want library.url", change.Key)
		}
	default:
		t.Fatal("authority change did not notify the live-value watcher")
	}
}

func TestSetDBPreservesEnvironmentOverrideOwnership(t *testing.T) {
	loader := testkit.NewSnapshotLoader(Snapshot{
		Values:       map[string]string{"library.url": "http://old:8096"},
		EnvOverrides: map[string]bool{"library.url": true},
	}, cloneSnapshot)
	svc, err := New(context.Background(), NewRegistry(), loader, nil)
	if err != nil {
		t.Fatal(err)
	}

	svc.SetDB(map[string]string{"library.url": "http://new:8096"})

	if !svc.IsUnlocked("library.url") {
		t.Fatal("values-only publication cleared environment ownership")
	}
	if got := svc.Resolve("library.url").Value; got != "http://new:8096" {
		t.Fatalf("library.url = %v, want values-only update", got)
	}
}

func TestReplicaRefresh_CannotPublishOldReadAfterLocalPatch(t *testing.T) {
	loader := testkit.NewSnapshotLoader(
		Snapshot{Values: map[string]string{"job.workers": "2"}},
		cloneSnapshot,
	)
	svc, err := New(context.Background(), NewRegistry(), loader, nil)
	if err != nil {
		t.Fatal(err)
	}
	loaded, release := loader.BlockNextRead()
	backgroundDone := make(chan error, 1)
	go func() { backgroundDone <- svc.Refresh(context.Background()) }()
	<-loaded // the background refresh captured the old generation and now blocks

	patchDone := make(chan error, 1)
	applied := make(chan struct{}, 1)
	go func() {
		persister := testkit.ApplyFunc[PersistenceBatch](func(_ context.Context, batch PersistenceBatch) error {
			snapshot := loader.Value()
			for _, row := range batch.Upserts {
				snapshot.Values[row.Key] = row.Value
			}
			for _, key := range batch.Deletes {
				delete(snapshot.Values, key)
			}
			loader.Set(snapshot, nil)
			applied <- struct{}{}
			return nil
		})
		_, patchErr := svc.Patch(context.Background(), persister,
			map[string]string{"job.workers": "7"}, "operator")
		patchDone <- patchErr
	}()
	<-applied
	release()
	if err := <-backgroundDone; err != nil {
		t.Fatal(err)
	}
	if err := <-patchDone; err != nil {
		t.Fatal(err)
	}
	if got := svc.Resolve("job.workers").Value; got != 7 {
		t.Fatalf("snapshot rolled back to %v after local PATCH, want 7", got)
	}
}
