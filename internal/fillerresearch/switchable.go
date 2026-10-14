package fillerresearch

import (
	"context"
	"fmt"
	"strings"
)

const SwitchableAdapterVersion = "switchable-v1"

// Switchable keeps the source-selection policy outside each retrieval adapter. Its identity
// changes with the live setting so a newly enabled source is eligible to refresh existing reports.
type Switchable struct {
	retriever Retriever
	enabled   func() bool
}

func NewSwitchable(retriever Retriever, enabled func() bool) (*Switchable, error) {
	if retriever == nil || enabled == nil {
		return nil, fmt.Errorf("%w: switchable retrieval requires an adapter and enabled callback", ErrInvalid)
	}
	adapter, version := retriever.Identity()
	if strings.TrimSpace(adapter) == "" || strings.TrimSpace(version) == "" {
		return nil, fmt.Errorf("%w: retrieval adapter identity is required", ErrInvalid)
	}
	return &Switchable{retriever: retriever, enabled: enabled}, nil
}

func (s *Switchable) Identity() (string, string) {
	if s == nil || s.retriever == nil {
		return "switchable", SwitchableAdapterVersion + ":unavailable"
	}
	adapter, version := s.retriever.Identity()
	state := "off"
	if s.enabled != nil && s.enabled() {
		state = "on"
	}
	return adapter, SwitchableAdapterVersion + ":" + version + ":" + state
}

func (s *Switchable) Retrieve(ctx context.Context, lookup Lookup) (Packet, error) {
	if s == nil || s.retriever == nil || s.enabled == nil {
		return Packet{}, fmt.Errorf("%w: switchable retrieval is unavailable", ErrInvalid)
	}
	if !s.enabled() {
		adapter, _ := s.retriever.Identity()
		return Packet{}, fmt.Errorf("%w: %s", ErrRetrieverDisabled, adapter)
	}
	return s.retriever.Retrieve(ctx, lookup)
}
