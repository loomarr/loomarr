package api

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
	"github.com/loomarr/loomarr/internal/testkit"
)

// #2736: the settings audit line read "Changed by" and a raw 32-character user id. The list
// names the author, marks the caller's own change as theirs, and never hands back a raw id.
func TestSettingsAuditNamesTheAuthorAndNeverAnID(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	now := time.Now()
	for _, u := range []identitystore.User{
		{ID: "90000001aaaaaaaaaaaaaaaaaaaaaaaa", Name: "Sam", Role: identitystore.RoleAdmin, CreatedAt: now, UpdatedAt: now},
		{ID: "90000002bbbbbbbbbbbbbbbbbbbbbbbb", Name: "Alex", Role: identitystore.RoleAdmin, CreatedAt: now, UpdatedAt: now},
	} {
		if err := st.UpsertUser(context.Background(), u); err != nil {
			t.Fatal(err)
		}
	}
	s := &Server{store: st}
	ctx := context.WithValue(context.Background(), userCtxKey{}, identitystore.User{ID: "90000002bbbbbbbbbbbbbbbbbbbbbbbb"})

	got, err := s.withAuditNames(ctx, []SettingEntry{
		{Key: "a", UpdatedBy: "90000001aaaaaaaaaaaaaaaaaaaaaaaa"},
		{Key: "b", UpdatedBy: "90000002bbbbbbbbbbbbbbbbbbbbbbbb"},
		{Key: "c", UpdatedBy: "90000003cccccccccccccccccccccccc"},
		{Key: "d"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].UpdatedBy != "Sam" || got[0].UpdatedByYou {
		t.Errorf("a = %+v, want named Sam and not the caller", got[0])
	}
	if got[1].UpdatedBy != "Alex" || !got[1].UpdatedByYou {
		t.Errorf("b = %+v, want the caller's own change", got[1])
	}
	if got[2].UpdatedBy != "" || got[2].UpdatedByYou {
		t.Errorf("c = %+v, an unknown author must be unnamed, never the raw id", got[2])
	}
	if got[3].UpdatedBy != "" || got[3].UpdatedByYou {
		t.Errorf("d = %+v, an unwritten key has no author", got[3])
	}
}
