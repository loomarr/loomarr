// Package libraryfixture provides no-network adapters for library-facing tests.
// It is separate from the root testkit package so it can use library domain types
// without creating a library <-> testkit import cycle.
package libraryfixture

import (
	"context"

	"github.com/loomarr/loomarr/internal/library/mediaserver"
)

// MediaSourceLibrary is one immutable media-source snapshot for setup tests.
type MediaSourceLibrary struct {
	ConnectionValue mediaserver.Connection
	Users           []mediaserver.User
	ListUsersErr    error
}

// Connection returns the connection carried by this snapshot.
func (m MediaSourceLibrary) Connection() mediaserver.Connection { return m.ConnectionValue }

// ListUsers returns the users carried by this snapshot without network access.
func (m MediaSourceLibrary) ListUsers(context.Context) ([]mediaserver.User, error) {
	return append([]mediaserver.User(nil), m.Users...), m.ListUsersErr
}
