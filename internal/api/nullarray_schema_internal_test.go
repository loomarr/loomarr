package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
)

// TestResponseSchemas_RequiredArraysNeverNull is the TYPE-driven half of the no-null contract;
// TestResponses_ContainNoJSONNull is the state-driven half.
//
// ⚠ The state-driven guard cannot see a nested field. It drives GETs against an EMPTY store, so
// every list answers `[]` and no element is ever encoded: a proposal whose `acquisitions` was nil
// (an idea request builds one without acquisitions) put `null` on the wire under a spec that
// says "required array", and the phone's Requests tab crashed calling `.length` on it — with
// that guard green. Filling the store would only move the blind spot to whichever branch the
// fixture didn't take.
//
// So this asks the question of every TYPE instead of every state: for each schema a response
// can reach, encode the Go type's zero value through the served JSON format and require that
// no required property comes out `null`. A zero value is the worst case a handler can hand
// the encoder, and each nested struct is its own registered schema, so every level is covered
// without constructing a single fixture.
func TestResponseSchemas_RequiredArraysNeverNull(t *testing.T) {
	_, humaAPI := schemaOnlyAPI(slog.New(slog.DiscardHandler))
	oapi := humaAPI.OpenAPI()
	registry := oapi.Components.Schemas
	// humaConfig is the one constructor behind the served API and the spec export, so this is
	// the encoder a client actually receives.
	format := humaConfig().Formats["application/json"]

	refs := responseSchemaRefs(oapi, registry)
	if len(refs) == 0 {
		t.Fatal("derived zero response schemas from the spec — the derivation broke, not the API")
	}
	checked := 0
	for _, ref := range refs {
		typ := registry.TypeFromRef(ref)
		schema := registry.SchemaFromRef(ref)
		if typ == nil {
			// A wire alias (durationwire.go) is a scalar with no Go struct behind it. An object
			// schema with no type would be a hole in this guard, not an exemption.
			if len(schema.Properties) > 0 {
				t.Errorf("%s has properties but no Go type to encode — the guard cannot check it", ref)
			}
			continue
		}
		checked++
		name := strings.TrimPrefix(ref, "#/components/schemas/")
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := format.Marshal(&buf, reflect.New(typ).Elem().Interface()); err != nil {
				t.Fatalf("encode zero %s: %v", typ, err)
			}
			var decoded map[string]any
			if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
				return // not an object on the wire (custom marshaller) — nothing to assert
			}
			for _, prop := range schema.Required {
				if p := schema.Properties[prop]; p != nil && p.Nullable {
					continue // declared `T | null` (a *float64 score): null is the contract
				}
				if v, ok := decoded[prop]; ok && v == nil {
					t.Errorf("%s.%s (%s) encodes as null, but the spec marks it required and "+
						"non-nullable — a client reading it as %s crashes",
						typ, prop, propType(schema, prop), propType(schema, prop))
				}
			}
		})
	}
	if checked == 0 {
		t.Fatal("resolved no response schema to a Go type — the guard checked nothing")
	}
}

// responseSchemaRefs returns every component schema reachable from an operation's response
// body, sorted. Request-only schemas are excluded: a client never decodes them.
func responseSchemaRefs(oapi *huma.OpenAPI, registry huma.Registry) []string {
	seen := map[string]bool{}
	var walk func(s *huma.Schema)
	walk = func(s *huma.Schema) {
		if s == nil {
			return
		}
		if s.Ref != "" {
			if seen[s.Ref] {
				return
			}
			seen[s.Ref] = true
			walk(registry.SchemaFromRef(s.Ref))
			return
		}
		walk(s.Items)
		for _, p := range s.Properties {
			walk(p)
		}
		if extra, ok := s.AdditionalProperties.(*huma.Schema); ok {
			walk(extra)
		}
		for _, group := range [][]*huma.Schema{s.OneOf, s.AnyOf, s.AllOf} {
			for _, sub := range group {
				walk(sub)
			}
		}
	}
	for _, item := range oapi.Paths {
		for _, op := range []*huma.Operation{item.Get, item.Put, item.Post, item.Delete, item.Patch} {
			if op == nil {
				continue
			}
			for _, resp := range op.Responses {
				for _, media := range resp.Content {
					walk(media.Schema)
				}
			}
		}
	}
	refs := make([]string, 0, len(seen))
	for ref := range seen {
		refs = append(refs, ref)
	}
	slices.Sort(refs)
	return refs
}

func propType(s *huma.Schema, prop string) string {
	p := s.Properties[prop]
	if p == nil {
		return "unknown"
	}
	if p.Type == "array" {
		return "an array"
	}
	if p.Type != "" {
		return "a " + p.Type
	}
	return "an object"
}
