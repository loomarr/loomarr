package testkit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// TunarrHTTPConfig scripts the endpoint-owned values exposed by TunarrHTTP.
type TunarrHTTPConfig struct {
	TranscodeConfigID             string
	ProgramID                     string
	FillerProgramID               string
	BeforeTranscodeConfigResponse func()
}

// TunarrFillerPolicy is one filler attachment observed by the shared Tunarr wire fixture.
type TunarrFillerPolicy struct {
	Weight          int
	CooldownSeconds int
}

// TunarrHTTP is the shared HTTP-level Tunarr fixture for adapter contract tests.
// It models the live-config endpoints as one stateful service so packages do not
// grow private service mocks for each configuration regression.
type TunarrHTTP struct {
	*httptest.Server

	t   testing.TB
	cfg TunarrHTTPConfig

	mu                  sync.Mutex
	requests            int
	transcodeConfigHits int
	createdTranscodeIDs []string
	programmedIDs       map[string][]string
	fillerListName      string
	fillerPrograms      []string
	fillerPolicies      map[string][]TunarrFillerPolicy
}

// NewTunarrHTTP starts a shared, stateful Tunarr adapter fixture.
func NewTunarrHTTP(t testing.TB, cfg TunarrHTTPConfig) *TunarrHTTP {
	t.Helper()
	m := &TunarrHTTP{
		t:              t,
		cfg:            cfg,
		programmedIDs:  map[string][]string{},
		fillerPolicies: map[string][]TunarrFillerPolicy{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/transcode_configs", m.handleTranscodeConfigs)
	mux.HandleFunc("POST /api/channels", m.handleCreateChannel)
	mux.HandleFunc("GET /api/media-sources", m.handleMediaSources)
	mux.HandleFunc("GET /api/media-sources/src-program/libraries", m.handleProgramLibraries)
	mux.HandleFunc("GET /api/media-sources/src-filler", m.handleFillerSource)
	mux.HandleFunc("GET /api/media-libraries/lib-program/programs", m.handlePrograms)
	mux.HandleFunc("GET /api/media-libraries/lib-filler/programs", m.handleFillerPrograms)
	mux.HandleFunc("POST /api/channels/{id}/programming", m.handleProgramming)
	mux.HandleFunc("GET /api/filler-lists", m.handleFillerLists)
	mux.HandleFunc("POST /api/filler-lists", m.handleWriteFillerList)
	mux.HandleFunc("PUT /api/filler-lists/{id}", m.handleWriteFillerList)
	mux.HandleFunc("GET /api/filler-lists/{id}/programs", m.handleFillerListPrograms)
	mux.HandleFunc("GET /api/channels/{id}", m.handleGetChannel)
	mux.HandleFunc("PUT /api/channels/{id}", m.handlePutChannel)
	m.Server = httptest.NewServer(mux)
	t.Cleanup(m.Close)
	return m
}

// RequestCount reports every request received by this Tunarr instance.
func (m *TunarrHTTP) RequestCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.requests
}

// TranscodeConfigHits reports default-transcode lookups.
func (m *TunarrHTTP) TranscodeConfigHits() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.transcodeConfigHits
}

// CreatedTranscodeIDs returns the transcode ids carried by channel creates.
func (m *TunarrHTTP) CreatedTranscodeIDs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.createdTranscodeIDs...)
}

// ProgrammedIDs returns the content ids last programmed on channelID.
func (m *TunarrHTTP) ProgrammedIDs(channelID string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.programmedIDs[channelID]...)
}

// FillerPolicies returns every policy attached to channelID, in write order.
func (m *TunarrHTTP) FillerPolicies(channelID string) []TunarrFillerPolicy {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]TunarrFillerPolicy(nil), m.fillerPolicies[channelID]...)
}

func (m *TunarrHTTP) observed() {
	m.mu.Lock()
	m.requests++
	m.mu.Unlock()
}

func (m *TunarrHTTP) handleTranscodeConfigs(w http.ResponseWriter, _ *http.Request) {
	m.observed()
	m.mu.Lock()
	m.transcodeConfigHits++
	m.mu.Unlock()
	if m.cfg.BeforeTranscodeConfigResponse != nil {
		m.cfg.BeforeTranscodeConfigResponse()
	}
	if m.cfg.TranscodeConfigID == "" {
		_ = json.NewEncoder(w).Encode([]any{})
		return
	}
	_ = json.NewEncoder(w).Encode([]map[string]string{{"id": m.cfg.TranscodeConfigID, "name": "Default"}})
}

func (m *TunarrHTTP) handleCreateChannel(w http.ResponseWriter, r *http.Request) {
	m.observed()
	var body struct {
		Channel struct {
			TranscodeID string `json:"transcodeConfigId"`
		} `json:"channel"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		m.t.Errorf("decode Tunarr channel create: %v", err)
	}
	m.mu.Lock()
	m.createdTranscodeIDs = append(m.createdTranscodeIDs, body.Channel.TranscodeID)
	m.mu.Unlock()
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"id":"created"}`))
}

func (m *TunarrHTTP) handleMediaSources(w http.ResponseWriter, _ *http.Request) {
	m.observed()
	var sources []map[string]any
	if m.cfg.ProgramID != "" {
		sources = append(sources, map[string]any{"id": "src-program", "type": "emby"})
	}
	if m.cfg.FillerProgramID != "" {
		sources = append(sources, map[string]any{"id": "src-filler", "type": "local"})
	}
	_ = json.NewEncoder(w).Encode(sources)
}

func (m *TunarrHTTP) handleProgramLibraries(w http.ResponseWriter, _ *http.Request) {
	m.observed()
	_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "lib-program", "enabled": true}})
}

func (m *TunarrHTTP) handleFillerSource(w http.ResponseWriter, _ *http.Request) {
	m.observed()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": "src-filler", "type": "local",
		"libraries": []map[string]any{{"id": "lib-filler"}},
	})
}

func (m *TunarrHTTP) handlePrograms(w http.ResponseWriter, _ *http.Request) {
	m.observed()
	_ = json.NewEncoder(w).Encode([]map[string]any{{
		"id": m.cfg.ProgramID,
		"program": map[string]any{
			"identifiers": []map[string]string{{"type": "emby", "id": "library-item"}},
		},
	}})
}

func (m *TunarrHTTP) handleFillerPrograms(w http.ResponseWriter, _ *http.Request) {
	m.observed()
	_ = json.NewEncoder(w).Encode([]map[string]any{{
		"id":      m.cfg.FillerProgramID,
		"program": map[string]any{"title": "Clip", "duration": 30_000},
	}})
}

func (m *TunarrHTTP) handleProgramming(w http.ResponseWriter, r *http.Request) {
	m.observed()
	var body struct {
		Lineup []struct {
			ID string `json:"id"`
		} `json:"lineup"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		m.t.Errorf("decode Tunarr programming body: %v", err)
	}
	ids := make([]string, len(body.Lineup))
	for i, item := range body.Lineup {
		ids[i] = item.ID
	}
	m.mu.Lock()
	m.programmedIDs[r.PathValue("id")] = ids
	m.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func (m *TunarrHTTP) handleFillerLists(w http.ResponseWriter, _ *http.Request) {
	m.observed()
	m.mu.Lock()
	name := m.fillerListName
	programs := append([]string(nil), m.fillerPrograms...)
	m.mu.Unlock()
	if len(programs) == 0 {
		_ = json.NewEncoder(w).Encode([]any{})
		return
	}
	_ = json.NewEncoder(w).Encode([]map[string]any{{
		"id": "list", "name": name, "contentCount": len(programs),
	}})
}

func (m *TunarrHTTP) handleWriteFillerList(w http.ResponseWriter, r *http.Request) {
	m.observed()
	var body struct {
		Name     string `json:"name"`
		Programs []struct {
			ID string `json:"id"`
		} `json:"programs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		m.t.Errorf("decode Tunarr filler-list body: %v", err)
	}
	programs := make([]string, len(body.Programs))
	for i, item := range body.Programs {
		programs[i] = item.ID
	}
	m.mu.Lock()
	m.fillerListName = body.Name
	m.fillerPrograms = programs
	m.mu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]string{"id": "list"})
}

func (m *TunarrHTTP) handleFillerListPrograms(w http.ResponseWriter, _ *http.Request) {
	m.observed()
	m.mu.Lock()
	programs := append([]string(nil), m.fillerPrograms...)
	m.mu.Unlock()
	rows := make([]map[string]string, len(programs))
	for i, id := range programs {
		rows[i] = map[string]string{"id": id}
	}
	_ = json.NewEncoder(w).Encode(rows)
}

func (m *TunarrHTTP) handleGetChannel(w http.ResponseWriter, r *http.Request) {
	m.observed()
	m.mu.Lock()
	history := append([]TunarrFillerPolicy(nil), m.fillerPolicies[r.PathValue("id")]...)
	m.mu.Unlock()
	collections := []any{}
	if len(history) > 0 {
		current := history[len(history)-1]
		collections = append(collections, map[string]any{
			"id": "list", "weight": current.Weight, "cooldownSeconds": current.CooldownSeconds,
		})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": r.PathValue("id"), "fillerCollections": collections,
	})
}

func (m *TunarrHTTP) handlePutChannel(w http.ResponseWriter, r *http.Request) {
	m.observed()
	var body struct {
		Collections []struct {
			Weight          int `json:"weight"`
			CooldownSeconds int `json:"cooldownSeconds"`
		} `json:"fillerCollections"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		m.t.Errorf("decode Tunarr channel update: %v", err)
	}
	if len(body.Collections) != 1 {
		m.t.Errorf("Tunarr channel %s filler collections = %+v, want one", r.PathValue("id"), body.Collections)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	policy := TunarrFillerPolicy{
		Weight: body.Collections[0].Weight, CooldownSeconds: body.Collections[0].CooldownSeconds,
	}
	m.mu.Lock()
	m.fillerPolicies[r.PathValue("id")] = append(m.fillerPolicies[r.PathValue("id")], policy)
	m.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}
