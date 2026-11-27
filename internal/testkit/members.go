package testkit

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
)

// SaveMembers saves a member for each id, named after it. Columns such as a filler pull's
// decided_by are foreign keys to users (#2793): an id that is no member is stored as no one, so a
// test that names people saves them first.
func SaveMembers(t testing.TB, st interface {
	UpsertUser(context.Context, identitystore.User) error
}, ids ...string,
) {
	t.Helper()
	for _, id := range ids {
		if err := st.UpsertUser(context.Background(), identitystore.User{ID: id, Name: id, Role: identitystore.RoleMember}); err != nil {
			t.Fatalf("save member %s: %v", id, err)
		}
	}
}
