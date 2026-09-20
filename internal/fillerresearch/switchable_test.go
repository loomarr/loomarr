package fillerresearch

import (
	"errors"
	"testing"
)

func TestSwitchableUsesLiveSettingAndChangesIdentity(t *testing.T) {
	enabled := true
	inner := &fixtureRetriever{packet: researchPacket()}
	retriever, err := NewSwitchable(inner, func() bool { return enabled })
	if err != nil {
		t.Fatal(err)
	}
	adapter, version := retriever.Identity()
	if adapter != "fixture" || version != "switchable-v1:fixture-v1:on" {
		t.Fatalf("enabled identity = %q %q", adapter, version)
	}
	if _, err := retriever.Retrieve(t.Context(), Lookup{Title: "Tootsie Pop"}); err != nil || inner.calls != 1 {
		t.Fatalf("enabled retrieve calls=%d err=%v", inner.calls, err)
	}

	enabled = false
	_, version = retriever.Identity()
	if version != "switchable-v1:fixture-v1:off" {
		t.Fatalf("disabled identity version = %q", version)
	}
	if _, err := retriever.Retrieve(t.Context(), Lookup{Title: "Tootsie Pop"}); !errors.Is(err, ErrRetrieverDisabled) || inner.calls != 1 {
		t.Fatalf("disabled retrieve calls=%d err=%v", inner.calls, err)
	}
}

func TestSwitchableRequiresAnIdentifiedAdapterAndCallback(t *testing.T) {
	if _, err := NewSwitchable(nil, func() bool { return true }); err == nil {
		t.Fatal("nil adapter accepted")
	}
	if _, err := NewSwitchable(&fixtureRetriever{}, nil); err == nil {
		t.Fatal("nil callback accepted")
	}
}
