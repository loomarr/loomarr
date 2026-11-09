package acquisition_test

import (
	"testing"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
)

func TestRemoteIdentityKey_NormalizesOnlyProvider(t *testing.T) {
	upper := acquisition.RemoteIdentity{Provider: " YouTube ", SourceID: "ChannelA", RemoteID: "AbCd123"}
	lowerProvider := acquisition.RemoteIdentity{Provider: "youtube", SourceID: "ChannelA", RemoteID: "AbCd123"}
	differentItemCase := acquisition.RemoteIdentity{Provider: "youtube", SourceID: "ChannelA", RemoteID: "abcd123"}
	if upper.Key() != lowerProvider.Key() {
		t.Fatalf("provider normalization keys = %q, %q; want equal", upper.Key(), lowerProvider.Key())
	}
	if upper.Key() == differentItemCase.Key() {
		t.Fatalf("case-sensitive remote ids share key %q", upper.Key())
	}
}
