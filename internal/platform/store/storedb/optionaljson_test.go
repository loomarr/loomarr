package storedb

import "testing"

// An absent document is NULL on the way in and "" on the way out; a present one passes through.
func TestOptionalJSONMapsAbsenceToNull(t *testing.T) {
	if v, err := OptionalJSON("").Value(); err != nil || v != nil {
		t.Fatalf(`OptionalJSON("").Value() = %v, %v; want nil`, v, err)
	}
	if v, err := OptionalJSON(`{"a":1}`).Value(); err != nil || v != `{"a":1}` {
		t.Fatalf("Value() = %v, %v; want the document", v, err)
	}
	for _, src := range []any{nil, `{"a":1}`, []byte(`[1]`)} {
		var j OptionalJSON = "stale"
		if err := j.Scan(src); err != nil {
			t.Fatalf("Scan(%#v): %v", src, err)
		}
		want := ""
		switch v := src.(type) {
		case string:
			want = v
		case []byte:
			want = string(v)
		}
		if string(j) != want {
			t.Fatalf("Scan(%#v) = %q, want %q", src, j, want)
		}
	}
	var j OptionalJSON
	if err := j.Scan(42); err == nil {
		t.Fatal("Scan(42) accepted a non-text value")
	}
}
