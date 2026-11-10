package testkit

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store"
)

// SaveMembers saves a member for each id, named after it. Who asked for a job or proposal, who
// decided it and who gave feedback are foreign keys to users (#2793): an id that is no member is
// stored as no one, so a test that names people saves them first.
func SaveMembers(t testing.TB, st interface {
	UpsertUser(context.Context, store.User) error
}, ids ...string,
) {
	t.Helper()
	for _, id := range ids {
		if err := st.UpsertUser(context.Background(), store.User{ID: id, Name: id, Role: store.RoleMember}); err != nil {
			t.Fatalf("save member %s: %v", id, err)
		}
	}
}
