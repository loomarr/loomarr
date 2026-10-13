package api

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

type wireRow struct {
	Name  string            `json:"name"`
	Tags  []string          `json:"tags"`
	Extra []string          `json:"extra,omitempty"`
	Attrs map[string]string `json:"attrs"`
	At    time.Time         `json:"at"`
	Raw   json.RawMessage   `json:"raw,omitempty"`
	Any   any               `json:"any"`
}

type wireTree struct {
	Label    string      `json:"label"`
	Children []*wireTree `json:"children"`
	Parent   *wireTree   `json:"-"`
}

func wireEncode(t *testing.T, v any) string {
	t.Helper()
	var buf bytes.Buffer
	if err := wireJSONFormat.Marshal(&buf, v); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// Nil slices and maps go out empty at every depth — through pointers, slices, maps and `any` —
// while omitempty still omits, a type's own MarshalJSON keeps its form, and the handler's value
// is never written to (it may be shared or cached).
func TestWireJSONSendsNilSlicesAndMapsEmpty(t *testing.T) {
	t.Parallel()
	shared := []wireRow{{Name: "a"}}
	body := struct {
		Rows  []wireRow           `json:"rows"`
		ByID  map[string]*wireRow `json:"byId"`
		None  []int               `json:"none"`
		Inner any                 `json:"inner"`
	}{Rows: shared, ByID: map[string]*wireRow{"b": {Name: "b", Tags: []string{"x"}}}, Inner: wireRow{Name: "c"}}

	got := wireEncode(t, body)
	want := `{"rows":[{"name":"a","tags":[],"attrs":{},"at":"0001-01-01T00:00:00Z","any":null}],` +
		`"byId":{"b":{"name":"b","tags":["x"],"attrs":{},"at":"0001-01-01T00:00:00Z","any":null}},` +
		`"none":[],"inner":{"name":"c","tags":[],"attrs":{},"at":"0001-01-01T00:00:00Z","any":null}}` + "\n"
	if got != want {
		t.Fatalf("encoded\n%s\nwant\n%s", got, want)
	}
	if shared[0].Tags != nil || shared[0].Attrs != nil || body.ByID["b"].Attrs != nil {
		t.Fatal("encoding wrote into the handler's value; the walk must copy, not mutate")
	}
}

// A top-level nil slice is a list response with nothing in it.
func TestWireJSONTopLevelNilSlice(t *testing.T) {
	t.Parallel()
	var rows []wireRow
	if got := wireEncode(t, rows); got != "[]\n" {
		t.Fatalf("nil top-level slice = %q, want []", got)
	}
}

type WireBase struct {
	Shared []string `json:"shared"`
}

// An exported embed's promoted fields are emptied; `omitzero` keeps meaning omitted; arrays and
// pointer chains are walked; a nil []byte is the empty base64 string its schema describes.
func TestWireJSONShapes(t *testing.T) {
	t.Parallel()
	body := struct {
		WireBase
		Gone  []int            `json:"gone,omitzero"`
		Pair  [2][]int         `json:"pair"`
		Deep  **[]string       `json:"deep"`
		Bytes []byte           `json:"bytes"`
		Grid  [][]int          `json:"grid"`
		Named map[string][]int `json:"named"`
	}{Grid: [][]int{nil}, Named: map[string][]int{"a": nil}}
	inner := (*[]string)(nil)
	body.Deep = &inner
	want := `{"shared":[],"pair":[[],[]],"deep":null,"bytes":"","grid":[[]],"named":{"a":[]}}` + "\n"
	if got := wireEncode(t, body); got != want {
		t.Fatalf("encoded\n%s\nwant\n%s", got, want)
	}
}

// A self-referential type terminates (its back-pointer is json:"-", but the walk still sees it).
func TestWireJSONRecursiveType(t *testing.T) {
	t.Parallel()
	root := &wireTree{Label: "root"}
	root.Children = []*wireTree{{Label: "leaf", Parent: root}}
	if got := wireEncode(t, root); got != `{"label":"root","children":[{"label":"leaf","children":[]}]}`+"\n" {
		t.Fatalf("tree = %s", got)
	}
}
