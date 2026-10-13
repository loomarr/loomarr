package api

import (
	"encoding"
	"encoding/json"
	"io"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/danielgtaylor/huma/v2"
)

// wireJSONFormat is the API's JSON encoder: Huma's default, with every nil slice sent as `[]`
// and every nil map as `{}`.
//
// ⚠ The spec declares no array or map nullable (humaConfig's DefaultArrayNullable note), but
// encoding/json writes a Go nil slice as `null`. Every handler therefore had to remember
// make([]T, 0) for every list field at every depth — 115 required arrays across the response
// types, any of which a forgotten branch could put on the wire as `null`. One did: requesting a
// channel from an idea built a proposal with no acquisitions, the queue sent
// `"acquisitions": null`, and the phone's Requests tab crashed on `.length`. Normalising here
// makes the spec true by construction instead of by per-handler discipline (one shape is out of
// reach; see onWire).
// TestResponseSchemas_RequiredArraysNeverNull holds the encoder to it for every response type.
//
// ⚠ It sits on every response, so its cost is budgeted: BenchmarkWireJSON_Guide (the largest
// body) must stay within 10% of BenchmarkWireJSON_GuideDefault. A naive walk — reflect over
// every value, copy every slice — was 5× the encode itself there.
//
// (encoding/json/v2 encodes nil as empty by default, but is still behind GOEXPERIMENT=jsonv2.)
var wireJSONFormat = huma.Format{
	Marshal: func(w io.Writer, v any) error {
		return huma.DefaultJSONFormat.Marshal(w, emptyNotNull(v))
	},
	Unmarshal: huma.DefaultJSONFormat.Unmarshal,
}

// emptyNotNull returns v with every nil slice and map that would be encoded as null replaced by
// an empty one. Copy-on-write: v and everything it points to are never mutated (a handler may
// return cached or shared data), only the path down to a replaced value is copied, and a value
// with nothing to replace is returned as-is without allocating.
func emptyNotNull(v any) any {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return v
	}
	plan := wirePlanFor(rv.Type())
	if plan == nil {
		return v
	}
	if out, changed := plan.emptied(rv, 0); changed {
		return out.Interface()
	}
	return v
}

// wirePlan is the walk for one type, computed once: which parts of a value of that type can
// hold a nil that encoding/json would write as null. A nil *wirePlan means none can, so the
// walk never enters the value — the Guide's rows of scalars and omitempty genres cost nothing.
type wirePlan struct {
	kind reflect.Kind
	// elem is the plan for a slice, array or map element or a pointer's target; nil when no
	// element can change (a []string is filled when nil but its elements are never visited).
	elem *wirePlan
	// fields are a struct's fields that can change, in declaration order.
	fields []wireField
}

type wireField struct {
	index int
	plan  *wirePlan
	// omitEmpty marks an omitempty slice or map: encoding/json omits it when nil or empty, so it
	// is never filled — only a non-empty one's elements are walked.
	omitEmpty bool
}

// maxEmptyDepth stops the walk on a cyclic value; encoding/json then reports the cycle itself.
const maxEmptyDepth = 1000

func (p *wirePlan) emptied(v reflect.Value, depth int) (reflect.Value, bool) {
	if depth > maxEmptyDepth {
		return v, false
	}
	switch p.kind {
	case reflect.Slice:
		if v.IsNil() {
			return reflect.MakeSlice(v.Type(), 0, 0), true
		}
		return p.emptiedElems(v, depth)
	case reflect.Array:
		return p.emptiedElems(v, depth)
	case reflect.Map:
		if v.IsNil() {
			return reflect.MakeMapWithSize(v.Type(), 0), true
		}
		if p.elem == nil {
			return v, false
		}
		var out reflect.Value
		for iter := v.MapRange(); iter.Next(); {
			if elem, changed := p.elem.emptied(iter.Value(), depth+1); changed {
				if !out.IsValid() {
					out = reflect.MakeMapWithSize(v.Type(), v.Len())
					for all := v.MapRange(); all.Next(); {
						out.SetMapIndex(all.Key(), all.Value())
					}
				}
				out.SetMapIndex(iter.Key(), elem)
			}
		}
		return out, out.IsValid()
	case reflect.Pointer:
		if v.IsNil() {
			return v, false
		}
		elem, changed := p.elem.emptied(v.Elem(), depth+1)
		if !changed {
			return v, false
		}
		out := reflect.New(v.Type().Elem())
		out.Elem().Set(elem)
		return out, true
	case reflect.Interface:
		// The one plan decided per value: what an interface holds is only known at runtime.
		if v.IsNil() {
			return v, false
		}
		held := wirePlanFor(v.Elem().Type())
		if held == nil {
			return v, false
		}
		elem, changed := held.emptied(v.Elem(), depth+1)
		if !changed {
			return v, false
		}
		out := reflect.New(v.Type()).Elem()
		out.Set(elem)
		return out, true
	case reflect.Struct:
		var out reflect.Value
		for _, f := range p.fields {
			field := v.Field(f.index)
			if f.omitEmpty && field.Len() == 0 {
				continue
			}
			if emptied, changed := f.plan.emptied(field, depth+1); changed {
				if !out.IsValid() {
					out = reflect.New(v.Type()).Elem()
					out.Set(v)
				}
				out.Field(f.index).Set(emptied)
			}
		}
		return out, out.IsValid()
	}
	return v, false
}

// emptiedElems walks a slice's or array's elements, copying it only once one changes.
func (p *wirePlan) emptiedElems(v reflect.Value, depth int) (reflect.Value, bool) {
	if p.elem == nil {
		return v, false
	}
	var out reflect.Value
	for i := range v.Len() {
		elem, changed := p.elem.emptied(v.Index(i), depth+1)
		if !changed {
			continue
		}
		if !out.IsValid() {
			if v.Kind() == reflect.Array {
				out = reflect.New(v.Type()).Elem()
				out.Set(v)
			} else {
				out = reflect.MakeSlice(v.Type(), v.Len(), v.Len())
				reflect.Copy(out, v)
			}
		}
		out.Index(i).Set(elem)
	}
	return out, out.IsValid()
}

// onWire reports whether the walk should visit the field. Skipping `json:"-"` matters beyond
// cost: it is where a back-pointer lives, and following one would walk a cycle. An `omitzero`
// field is left alone because emptying it would change it from omitted to present.
//
// ⚠ An embedded UNEXPORTED struct is skipped although encoding/json promotes its exported
// fields: reflection cannot set through it. Its nil slices still go out as null, and
// TestResponseSchemas_RequiredArraysNeverNull fails on any response type that embeds one that
// way. Embed an exported type instead.
func onWire(f reflect.StructField) bool {
	tag := f.Tag.Get("json")
	return f.IsExported() && tag != "-" && !slices.Contains(strings.Split(tag, ",")[1:], "omitzero")
}

func omitsEmpty(f reflect.StructField) bool {
	return slices.Contains(strings.Split(f.Tag.Get("json"), ",")[1:], "omitempty")
}

var (
	jsonMarshaler = reflect.TypeFor[json.Marshaler]()
	textMarshaler = reflect.TypeFor[encoding.TextMarshaler]()
	// wirePlans caches wirePlanFor per type; the mutex only serialises building new entries,
	// which are published once complete so a reader never sees a half-built plan.
	wirePlans   sync.Map
	wirePlansMu sync.Mutex
)

// wirePlanFor returns t's cached plan, building it (and every type it reaches) on first use.
func wirePlanFor(t reflect.Type) *wirePlan {
	if known, ok := wirePlans.Load(t); ok {
		return known.(*wirePlan)
	}
	wirePlansMu.Lock()
	defer wirePlansMu.Unlock()
	built := map[reflect.Type]*wirePlan{}
	plan := buildWirePlan(t, built)
	for t, p := range built {
		wirePlans.Store(t, p)
	}
	return plan
}

// buildWirePlan computes t's plan into built. A type with its own MarshalJSON/MarshalText owns
// its wire form and gets none.
func buildWirePlan(t reflect.Type, built map[reflect.Type]*wirePlan) *wirePlan {
	if known, ok := wirePlans.Load(t); ok {
		return known.(*wirePlan)
	}
	if known, ok := built[t]; ok {
		// A recursive type meets itself mid-build and gets its own unfinished plan: assumed to
		// need a walk, which is what it resolves to — anything reaching it is walked too.
		return known
	}
	if t.Implements(jsonMarshaler) || t.Implements(textMarshaler) ||
		reflect.PointerTo(t).Implements(jsonMarshaler) || reflect.PointerTo(t).Implements(textMarshaler) {
		built[t] = nil
		return nil
	}
	plan := &wirePlan{kind: t.Kind()}
	built[t] = plan
	switch t.Kind() {
	case reflect.Slice, reflect.Map:
		plan.elem = buildWirePlan(t.Elem(), built)
	case reflect.Interface:
	case reflect.Pointer, reflect.Array:
		if plan.elem = buildWirePlan(t.Elem(), built); plan.elem == nil {
			plan = nil
		}
	case reflect.Struct:
		for i := range t.NumField() {
			f := t.Field(i)
			if !onWire(f) {
				continue
			}
			fp := buildWirePlan(f.Type, built)
			omitEmpty := (f.Type.Kind() == reflect.Slice || f.Type.Kind() == reflect.Map) && omitsEmpty(f)
			if fp == nil || (omitEmpty && fp.elem == nil) {
				continue
			}
			plan.fields = append(plan.fields, wireField{index: i, plan: fp, omitEmpty: omitEmpty})
		}
		if len(plan.fields) == 0 {
			plan = nil
		}
	default:
		plan = nil
	}
	built[t] = plan
	return plan
}
