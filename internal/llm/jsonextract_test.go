package llm_test

import (
	"testing"

	"github.com/loomarr/loomarr/internal/llm"
)

func TestExtractJSONObject(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"bare object unchanged", `{"a":1}`, `{"a":1}`},
		// ⚠ THE live case (§10 V44): llava:7b fences its answer. The leading backtick made
		// json.Unmarshal fail; the span between the fences must come back clean.
		{"markdown json fence", "```json\n{\"brand\":\"Ford\"}\n```", `{"brand":"Ford"}`},
		{"plain fence", "```\n{\"x\":true}\n```", `{"x":true}`},
		{"prose around it", `Sure! Here is the answer: {"era":1994} — hope that helps.`, `{"era":1994}`},
		{"nested braces", `{"a":{"b":2},"c":3}`, `{"a":{"b":2},"c":3}`},
		// A brace inside a string value must not end the object early.
		{"brace in string", `{"title":"a } b","n":1}`, `{"title":"a } b","n":1}`},
		// No object at all — returned unchanged so json.Unmarshal reports the real error.
		{"no object", `I cannot answer that.`, `I cannot answer that.`},
		{"unbalanced", `{"a":1`, `{"a":1`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := llm.ExtractJSONObject(c.in); got != c.want {
				t.Errorf("ExtractJSONObject(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
