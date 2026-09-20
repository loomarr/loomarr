package fillerresearch

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const FederatedAdapterVersion = "federated-v1"

// Federated is the retrieval implementation behind the package's small external interface. It
// keeps provider fan-out, partial-failure behavior, URL de-duplication and packet ceilings local.
type Federated struct {
	retrievers []Retriever
	version    string
}

func NewFederated(retrievers ...Retriever) (*Federated, error) {
	filtered := make([]Retriever, 0, len(retrievers))
	for _, retriever := range retrievers {
		if retriever != nil {
			filtered = append(filtered, retriever)
		}
	}
	if len(filtered) < 2 {
		return nil, fmt.Errorf("%w: federated retrieval requires at least two adapters", ErrInvalid)
	}
	versions := make([]string, 0, len(filtered))
	for _, retriever := range filtered {
		adapter, version := retriever.Identity()
		if strings.TrimSpace(adapter) == "" || strings.TrimSpace(version) == "" {
			return nil, fmt.Errorf("%w: retrieval adapter identity is required", ErrInvalid)
		}
		versions = append(versions, adapter+":"+version)
	}
	return &Federated{retrievers: filtered, version: FederatedAdapterVersion + ":" + strings.Join(versions, "+")}, nil
}

func (f *Federated) Identity() (string, string) {
	if f == nil {
		return "federated", FederatedAdapterVersion
	}
	return "federated", f.version
}

func (f *Federated) Retrieve(ctx context.Context, lookup Lookup) (Packet, error) {
	if f == nil || len(f.retrievers) == 0 {
		return Packet{}, fmt.Errorf("%w: retrieval is unavailable", ErrInvalid)
	}
	if err := lookup.Validate(); err != nil {
		return Packet{}, err
	}
	packet := Packet{Query: lookup.CanonicalTitle(), Adapter: "federated",
		AdapterVersion: FederatedAdapterVersion}
	var failures []error
	packets := make([]Packet, 0, len(f.retrievers))
	for _, retriever := range f.retrievers {
		if err := ctx.Err(); err != nil {
			return Packet{}, err
		}
		candidate, err := retriever.Retrieve(ctx, lookup)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if err := candidate.Validate(); err != nil {
			failures = append(failures, err)
			continue
		}
		if candidate.RetrievedAt.After(packet.RetrievedAt) {
			packet.RetrievedAt = candidate.RetrievedAt
		}
		packets = append(packets, candidate)
	}
	// Take one result from each successful peer before taking a second. Sequential append let the
	// first adapter consume three of five slots and made adding another source capable of starving
	// the peers that followed it.
	seen := make(map[string]bool, MaxCitations)
	for index := 0; len(packet.Citations) < MaxCitations; index++ {
		added := false
		for _, candidate := range packets {
			if index >= len(candidate.Citations) {
				continue
			}
			citation := candidate.Citations[index]
			key := strings.ToLower(strings.TrimSpace(citation.URL))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			citation.ID = len(packet.Citations) + 1
			packet.Citations = append(packet.Citations, citation)
			added = true
			if len(packet.Citations) == MaxCitations {
				break
			}
		}
		if !added {
			break
		}
	}
	if len(packet.Citations) == 0 {
		return Packet{}, fmt.Errorf("retrieve filler context evidence: %w", errors.Join(failures...))
	}
	packet.Citations = boundCitations(packet.Citations)
	packet.AdapterVersion = f.version
	if err := packet.Validate(); err != nil {
		return Packet{}, err
	}
	return packet, nil
}
