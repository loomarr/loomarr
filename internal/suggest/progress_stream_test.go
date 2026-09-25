package suggest

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

const streamReply = "Here you go:\n```json\n" + `{"channelName":"90s {Action}","rationale":"says \"fast\" } ok","picks":[` +
	`{"key":"movie:tmdb:100","rationale":"a \"bus\" that }{ can't stop","confidence":0.9},` +
	`{"key":"movie:tmdb:603","seasonMin":0,"confidence":0.8}` +
	`],"policy":{"rules":[{"when":"weekend"}],"genres":{"include":["a"]}},"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}` + "\n```"

func collectPicks(chunks []string) []string {
	var keys []string
	s := newPickStream(func(p pick) { keys = append(keys, p.Key) })
	for _, c := range chunks {
		s.Write(c)
	}
	return keys
}

func TestPickStream_WholeReplyYieldsPicksInOrder(t *testing.T) {
	got := collectPicks([]string{streamReply})
	if want := []string{"movie:tmdb:100", "movie:tmdb:603"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("picks = %v, want %v", got, want)
	}
}

// Every possible split point — including inside a string, between an escape's backslash and
// its character, and mid-key — must give the same answer as the whole reply.
func TestPickStream_EverySplitPointGivesTheSameAnswer(t *testing.T) {
	want := collectPicks([]string{streamReply})
	for i := 0; i <= len(streamReply); i++ {
		if got := collectPicks([]string{streamReply[:i], streamReply[i:]}); !reflect.DeepEqual(got, want) {
			t.Fatalf("split at %d: picks = %v, want %v", i, got, want)
		}
	}
	// And one byte at a time — the worst case a tokenizer can produce.
	var bytes []string
	for i := 0; i < len(streamReply); i++ {
		bytes = append(bytes, streamReply[i:i+1])
	}
	if got := collectPicks(bytes); !reflect.DeepEqual(got, want) {
		t.Fatalf("byte-at-a-time picks = %v, want %v", got, want)
	}
}

// A pick is emitted only when its object CLOSES, never earlier from a half-written entry.
func TestPickStream_IncompleteObjectIsNotEmitted(t *testing.T) {
	head := `{"picks":[{"key":"movie:tmdb:100","rationale":"a"},{"key":"movie:tmdb:603","rati`
	if got := collectPicks([]string{head}); !reflect.DeepEqual(got, []string{"movie:tmdb:100"}) {
		t.Fatalf("picks = %v, want only the closed first entry", got)
	}
}

// Nested arrays/objects elsewhere (policy.rules, genres) must never be mistaken for picks.
func TestPickStream_OnlyTopLevelPicksArrayEmits(t *testing.T) {
	reply := `{"policy":{"picks":[{"key":"movie:tmdb:1"}],"rules":[{"key":"movie:tmdb:2"}]},"picks":[{"key":"movie:tmdb:3"}]}`
	if got := collectPicks([]string{reply}); !reflect.DeepEqual(got, []string{"movie:tmdb:3"}) {
		t.Fatalf("picks = %v, want only the top-level entry", got)
	}
}

func TestPickStream_UnparseableEntryIsSkippedNotFatal(t *testing.T) {
	reply := `{"picks":[{"key":movie},{"key":"movie:tmdb:9"}]}`
	if got := collectPicks([]string{reply}); !reflect.DeepEqual(got, []string{"movie:tmdb:9"}) {
		t.Fatalf("picks = %v, want the later valid entry", got)
	}
}

// An escaped quote must not end the string: the brace after it is text, not the object's end.
func TestPickStream_EscapedQuoteDoesNotEndTheString(t *testing.T) {
	reply := `{"picks":[{"key":"movie:tmdb:1","rationale":"he said \"}\" loudly"},{"key":"movie:tmdb:2"}]}`
	if got := collectPicks([]string{reply}); !reflect.DeepEqual(got, []string{"movie:tmdb:1", "movie:tmdb:2"}) {
		t.Fatalf("picks = %v", got)
	}
}

// "4 of about 8 picked" is honest only while 8 is what the model is actually asked for.
func TestLineupPickCap_MatchesThePrompt(t *testing.T) {
	want := fmt.Sprintf("Select at most %d picks total", LineupPickCap)
	if !strings.Contains(systemPrompt, want) {
		t.Fatalf("systemPrompt no longer says %q; update LineupPickCap or the prompt together", want)
	}
}
