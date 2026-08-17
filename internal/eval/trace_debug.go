//go:build eval

package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/suggest"
)

const evalTracePrefix = "[DEBUG-eval-boundary]"

// Temporary diagnosis-only provider wrapper. It records query/response shapes
// and aggregate metadata signals, never endpoints, headers, credentials, title
// names, overview text, or library host information.
type evalTraceProvider struct {
	llm.Provider
	seenMessages int
	candidates   map[string]traceCandidate
}

type traceCandidate struct {
	inLibrary bool
	mystery   bool
	cozy      bool
	graphic   bool
}

func newEvalTraceProvider(provider llm.Provider) llm.Provider {
	return &evalTraceProvider{Provider: provider, candidates: make(map[string]traceCandidate)}
}

func (p *evalTraceProvider) Chat(ctx context.Context, messages []llm.Message, opts llm.ChatOptions) (llm.Response, error) {
	if len(messages) < p.seenMessages {
		p.seenMessages = 0
		p.candidates = make(map[string]traceCandidate)
	}
	for _, message := range messages[p.seenMessages:] {
		if message.Role != llm.Tool {
			continue
		}
		var candidates []struct {
			MediaType string   `json:"mediaType"`
			TMDBID    int      `json:"tmdbId"`
			TVDBID    int      `json:"tvdbId"`
			InLibrary bool     `json:"inLibrary"`
			Genres    []string `json:"genres"`
			Overview  string   `json:"overview"`
		}
		if err := json.Unmarshal([]byte(message.Content), &candidates); err != nil {
			fmt.Fprintf(os.Stderr, "%s tool-result shape=error bytes=%d\n", evalTracePrefix, len(message.Content))
			continue
		}
		inLibrary, mystery, cozy, graphic := 0, 0, 0, 0
		for _, candidate := range candidates {
			key := traceKey(candidate.MediaType, candidate.TMDBID, candidate.TVDBID)
			if key == "" {
				continue
			}
			signal := classifyCandidate(candidate.Genres, candidate.Overview)
			signal.inLibrary = candidate.InLibrary
			p.candidates[key] = signal
			inLibrary += boolInt(signal.inLibrary)
			mystery += boolInt(signal.mystery)
			cozy += boolInt(signal.cozy)
			graphic += boolInt(signal.graphic)
		}
		fmt.Fprintf(os.Stderr, "%s tool-result candidates=%d surfaced_total=%d in_library=%d mystery_signal=%d cozy_signal=%d graphic_signal=%d\n",
			evalTracePrefix, len(candidates), len(p.candidates), inLibrary, mystery, cozy, graphic)
	}
	p.seenMessages = len(messages)

	response, err := p.Provider.Chat(ctx, messages, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s model error=true\n", evalTracePrefix)
		return response, err
	}
	if response.WantsTools() {
		for _, call := range response.ToolCalls {
			args := map[string]any{}
			for _, key := range []string{"query", "genres", "era", "media_type"} {
				if value, ok := call.Arguments[key]; ok {
					args[key] = value
				}
			}
			encoded, _ := json.Marshal(args)
			fmt.Fprintf(os.Stderr, "%s tool-call name=%s args=%s\n", evalTracePrefix, call.Name, encoded)
		}
		return response, nil
	}
	var final struct {
		Picks []struct {
			MediaType string `json:"mediaType"`
			TMDBID    int    `json:"tmdbId"`
			TVDBID    int    `json:"tvdbId"`
		} `json:"picks"`
	}
	if err := json.Unmarshal([]byte(llm.ExtractJSONObject(response.Content)), &final); err != nil {
		fmt.Fprintf(os.Stderr, "%s final shape=invalid bytes=%d surfaced_total=%d\n", evalTracePrefix, len(response.Content), len(p.candidates))
		return response, nil
	}
	matched, invalid, inLibrary, mystery, cozy, graphic := 0, 0, 0, 0, 0, 0
	for _, pick := range final.Picks {
		key := traceKey(pick.MediaType, pick.TMDBID, pick.TVDBID)
		if key == "" {
			invalid++
			continue
		}
		signal, ok := p.candidates[key]
		if !ok {
			continue
		}
		matched++
		inLibrary += boolInt(signal.inLibrary)
		mystery += boolInt(signal.mystery)
		cozy += boolInt(signal.cozy)
		graphic += boolInt(signal.graphic)
	}
	fmt.Fprintf(os.Stderr, "%s final picks=%d matched=%d invalid=%d surfaced_total=%d selected_in_library=%d selected_mystery=%d selected_cozy=%d selected_graphic=%d bytes=%d\n",
		evalTracePrefix, len(final.Picks), matched, invalid, len(p.candidates), inLibrary, mystery, cozy, graphic, len(response.Content))
	return response, nil
}

type evalTraceValidator struct {
	inner suggest.Validator
}

func (v evalTraceValidator) Exists(ctx context.Context, mediaType provision.MediaType, tmdbID int) (bool, error) {
	ok, err := v.inner.Exists(ctx, mediaType, tmdbID)
	fmt.Fprintf(os.Stderr, "%s acquisition-validation media_type=%s id_present=%t exists=%t error=%t\n",
		evalTracePrefix, mediaType, tmdbID > 0, ok, err != nil)
	return ok, err
}

func traceKey(mediaType string, tmdbID, tvdbID int) string {
	if mediaType == string(provision.Series) && tvdbID > 0 {
		return fmt.Sprintf("%s:tvdb:%d", mediaType, tvdbID)
	}
	if tmdbID > 0 {
		return fmt.Sprintf("%s:tmdb:%d", mediaType, tmdbID)
	}
	if tvdbID > 0 {
		return fmt.Sprintf("%s:tvdb:%d", mediaType, tvdbID)
	}
	return ""
}

func classifyCandidate(genres []string, overview string) traceCandidate {
	text := strings.ToLower(strings.Join(genres, " ") + " " + overview)
	return traceCandidate{
		mystery: containsAny(text, "mystery", "detective", "investigat", "murder", "crime"),
		cozy:    containsAny(text, "cozy", "quaint", "village", "small town", "small-town", "amateur detective"),
		graphic: containsAny(text, "gruesome", "gore", "gory", "torture", "slasher", "serial killer", "brutal", "blood-soaked", "violent"),
	}
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
