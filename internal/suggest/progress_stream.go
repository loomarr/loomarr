package suggest

import (
	"encoding/json"
	"strings"
)

// pickStream extracts each COMPLETE entry of the final JSON's top-level "picks" array from a
// reply that arrives as arbitrary fragments (a token, half a key, a whole object). It is a
// small byte-level scanner, not a JSON parser: it tracks only string/escape state and
// container nesting, so a fragment boundary can fall anywhere — inside a string, between an
// escape's backslash and its character, mid-key — and the result is the same as feeding the
// whole reply at once. CPU only: no model call, no allocation beyond the buffered object.
//
// It never decides whether a pick is GOOD — resolvePick does, exactly as final validation
// does. It only says "this object is finished and parses"; an entry that does not parse
// (or arrives inside prose or a code fence we cannot read) is skipped, and the final parse
// stays the authority.
type pickStream struct {
	emit func(pick)

	stack    []byte // open containers: '{' or '['
	inString bool
	escaped  bool
	strBuf   strings.Builder // the current string's raw text, kept only at depth 1 (a key)
	keepStr  bool
	lastKey  string // most recent string closed at depth 1
	rootKey  string // key the next depth-1 value belongs to
	inPicks  bool   // inside the depth-1 "picks" array
	obj      strings.Builder
	inObj    bool
}

func newPickStream(emit func(pick)) *pickStream { return &pickStream{emit: emit} }

// Write feeds the next fragment of the reply.
func (s *pickStream) Write(fragment string) {
	for i := 0; i < len(fragment); i++ {
		s.step(fragment[i])
	}
}

func (s *pickStream) step(c byte) {
	if s.inObj {
		s.obj.WriteByte(c)
	}
	if s.inString {
		switch {
		case s.escaped:
			s.escaped = false
			if s.keepStr {
				s.strBuf.WriteByte(c)
			}
		case c == '\\':
			s.escaped = true
			if s.keepStr {
				s.strBuf.WriteByte(c)
			}
		case c == '"':
			s.inString = false
			if s.keepStr {
				s.lastKey = s.strBuf.String()
			}
		default:
			if s.keepStr {
				s.strBuf.WriteByte(c)
			}
		}
		return
	}
	switch c {
	case '"':
		s.inString = true
		s.keepStr = len(s.stack) == 1 && s.stack[0] == '{'
		if s.keepStr {
			s.strBuf.Reset()
		}
	case ':':
		if len(s.stack) == 1 {
			s.rootKey = s.lastKey
		}
	case '{':
		s.stack = append(s.stack, '{')
		// Depth 3 = root object → picks array → one pick object.
		if s.inPicks && len(s.stack) == 3 {
			s.inObj = true
			s.obj.Reset()
			s.obj.WriteByte('{')
		}
	case '[':
		s.stack = append(s.stack, '[')
		if len(s.stack) == 2 {
			s.inPicks = s.rootKey == "picks"
		}
	case '}', ']':
		if len(s.stack) == 0 {
			return
		}
		s.stack = s.stack[:len(s.stack)-1]
		if c == '}' && s.inObj && len(s.stack) == 2 {
			s.inObj = false
			var p pick
			if err := json.Unmarshal([]byte(s.obj.String()), &p); err == nil {
				s.emit(p)
			}
		}
		if c == ']' && len(s.stack) == 1 {
			s.inPicks = false
		}
	}
}
