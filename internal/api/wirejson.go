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
// (encoding/json/v2 encodes nil as empty by default, but is still behind GOEXPERIMENT=jsonv2.)
var wireJSONFormat = huma.Format{
	Marshal: func(w io.Writer, v any) error {
		return huma.DefaultJSONFormat.Marshal(w, emptyNotNull(v))
	},
	Unmarshal: huma.DefaultJSONFormat.Unmarshal,
}

// emptyNotNull returns v with every nil slice and map it reaches replaced by an empty one.
// Copy-on-write: v and everything it points to are never mutated (a handler may return cached
// or shared data), and a value with nothing to replace is returned as-is.
func emptyNotNull(v any) any {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() || !mayHoldNil(rv.Type()) {
		return v
	}
	if out, changed := emptied(rv, 0); changed {
		return out.Interface()
	}
	return v
}

// maxEmptyDepth stops the walk on a cyclic value; encoding/json then reports the cycle itself.
const maxEmptyDepth = 1000

func emptied(v reflect.Value, depth int) (reflect.Value, bool) {
	if depth > maxEmptyDepth || !mayHoldNil(v.Type()) {
		return v, false
	}
	switch v.Kind() {
	case reflect.Slice:
		if v.IsNil() {
			return reflect.MakeSlice(v.Type(), 0, 0), true
		}
		return emptiedElems(v, reflect.MakeSlice(v.Type(), v.Len(), v.Len()), depth)
	case reflect.Array:
		return emptiedElems(v, reflect.New(v.Type()).Elem(), depth)
	case reflect.Map:
		if v.IsNil() {
			return reflect.MakeMapWithSize(v.Type(), 0), true
		}
		var out reflect.Value
		for iter := v.MapRange(); iter.Next(); {
			if elem, changed := emptied(iter.Value(), depth+1); changed {
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
		elem, changed := emptied(v.Elem(), depth+1)
		if !changed {
			return v, false
		}
		out := reflect.New(elem.Type())
		out.Elem().Set(elem)
		return out, true
	case reflect.Interface:
		if v.IsNil() {
			return v, false
		}
		elem, changed := emptied(v.Elem(), depth+1)
		if !changed {
			return v, false
		}
		out := reflect.New(v.Type()).Elem()
		out.Set(elem)
		return out, true
	case reflect.Struct:
		var out reflect.Value
		for i := range v.NumField() {
			if !onWire(v.Type().Field(i)) {
				continue
			}
			if field, changed := emptied(v.Field(i), depth+1); changed {
				if !out.IsValid() {
					out = reflect.New(v.Type()).Elem()
					out.Set(v)
				}
				out.Field(i).Set(field)
			}
		}
		return out, out.IsValid()
	}
	return v, false
}

// emptiedElems copies src into dst (a fresh slice or array of the same length) only if some
// element changes; otherwise it reports src unchanged and dst is discarded.
func emptiedElems(src, dst reflect.Value, depth int) (reflect.Value, bool) {
	copied := false
	for i := range src.Len() {
		elem, changed := emptied(src.Index(i), depth+1)
		if !changed {
			continue
		}
		if !copied {
			reflect.Copy(dst, src)
			copied = true
		}
		dst.Index(i).Set(elem)
	}
	if !copied {
		return src, false
	}
	return dst, true
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

var (
	jsonMarshaler = reflect.TypeFor[json.Marshaler]()
	textMarshaler = reflect.TypeFor[encoding.TextMarshaler]()
	// mayHoldNilOf caches mayHoldNil per type; the mutex only serialises computing a new entry.
	mayHoldNilOf sync.Map
	mayHoldNilMu sync.Mutex
)

// mayHoldNil reports whether a value of t can reach a nil slice or map that encoding/json would
// write — the walk skips everything else, so a long list of scalar rows costs one cached lookup
// per element. A type with its own MarshalJSON/MarshalText owns its wire form and is left alone.
func mayHoldNil(t reflect.Type) bool {
	if known, ok := mayHoldNilOf.Load(t); ok {
		return known.(bool)
	}
	mayHoldNilMu.Lock()
	defer mayHoldNilMu.Unlock()
	return mayHoldNilLocked(t)
}

func mayHoldNilLocked(t reflect.Type) bool {
	if known, ok := mayHoldNilOf.Load(t); ok {
		return known.(bool)
	}
	// A recursive type meets itself mid-computation; answer conservatively (walk it) rather
	// than let a provisional "no" be cached into a type that refers back to this one.
	mayHoldNilOf.Store(t, true)
	result := false
	switch {
	case t.Implements(jsonMarshaler) || t.Implements(textMarshaler) ||
		reflect.PointerTo(t).Implements(jsonMarshaler) || reflect.PointerTo(t).Implements(textMarshaler):
	default:
		switch t.Kind() {
		case reflect.Slice, reflect.Map, reflect.Interface:
			result = true
		case reflect.Pointer, reflect.Array:
			result = mayHoldNilLocked(t.Elem())
		case reflect.Struct:
			for i := range t.NumField() {
				if f := t.Field(i); onWire(f) && mayHoldNilLocked(f.Type) {
					result = true
					break
				}
			}
		}
	}
	mayHoldNilOf.Store(t, result)
	return result
}
