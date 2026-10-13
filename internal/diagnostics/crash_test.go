package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"
)

// crashMemory is the persistence role in memory; the store's own behaviour is pinned by the
// conformance suite over SQLite and Postgres.
type crashMemory struct{ records []ClientCrashRecord }

func (m *crashMemory) InsertClientCrashReport(_ context.Context, r ClientCrashRecord) error {
	m.records = append(m.records, r)
	return nil
}

func (m *crashMemory) QueryClientCrashReports(_ context.Context, q ClientCrashStoreQuery) ([]ClientCrashRecord, error) {
	sorted := append([]ClientCrashRecord(nil), m.records...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].ReceivedAt != sorted[j].ReceivedAt {
			return sorted[i].ReceivedAt > sorted[j].ReceivedAt
		}
		return sorted[i].ID > sorted[j].ID
	})
	var out []ClientCrashRecord
	for _, r := range sorted {
		if q.CursorID != "" && (r.ReceivedAt > q.CursorReceivedAt ||
			(r.ReceivedAt == q.CursorReceivedAt && r.ID >= q.CursorID)) {
			continue
		}
		if len(out) == q.Limit {
			break
		}
		out = append(out, r)
	}
	return out, nil
}

func (m *crashMemory) GetClientCrashReport(_ context.Context, id string) (ClientCrashRecord, bool, error) {
	for _, r := range m.records {
		if r.ID == id {
			return r, true, nil
		}
	}
	return ClientCrashRecord{}, false, nil
}

func validCrash() ClientCrashReportInput {
	return ClientCrashReportInput{
		Kind: CrashJSFatal, Platform: CrashIOS, AppVersion: "1.4.0", AppBuild: "212",
		OSVersion: "26.1", DeviceModel: "iPhone17,3", OccurredAt: "2026-10-13T08:00:00Z",
		Message: "TypeError: undefined is not an object", Stack: "at render (App.tsx:12:3)",
		Breadcrumbs: []ClientCrashBreadcrumb{{At: "2026-10-13T07:59:58Z", Category: BreadcrumbNavigation, Message: "/watch/7"}},
	}
}

func TestClientCrashValidationRejectsEveryContractViolation(t *testing.T) {
	t.Parallel()
	tooMany := make([]ClientCrashBreadcrumb, 51)
	for i := range tooMany {
		tooMany[i] = ClientCrashBreadcrumb{At: "2026-10-13T07:59:58Z", Category: BreadcrumbLog, Message: "x"}
	}
	cases := map[string]func(*ClientCrashReportInput){
		"unknown kind":              func(in *ClientCrashReportInput) { in.Kind = "panic" },
		"unknown platform":          func(in *ClientCrashReportInput) { in.Platform = "windows" },
		"empty message":             func(in *ClientCrashReportInput) { in.Message = "  " },
		"message over 2 KiB":        func(in *ClientCrashReportInput) { in.Message = strings.Repeat("é", 1025) },
		"stack over 32 KiB":         func(in *ClientCrashReportInput) { in.Stack = strings.Repeat("s", 32<<10+1) },
		"label over 128 bytes":      func(in *ClientCrashReportInput) { in.DeviceModel = strings.Repeat("m", 129) },
		"occurredAt not RFC 3339":   func(in *ClientCrashReportInput) { in.OccurredAt = "yesterday" },
		"51 breadcrumbs":            func(in *ClientCrashReportInput) { in.Breadcrumbs = tooMany },
		"unknown breadcrumb":        func(in *ClientCrashReportInput) { in.Breadcrumbs[0].Category = "console" },
		"breadcrumb over 256 bytes": func(in *ClientCrashReportInput) { in.Breadcrumbs[0].Message = strings.Repeat("b", 257) },
		"breadcrumb time":           func(in *ClientCrashReportInput) { in.Breadcrumbs[0].At = "" },
		"native payload on js":      func(in *ClientCrashReportInput) { in.NativePayload = json.RawMessage(`{"a":1}`) },
		"native payload not object": func(in *ClientCrashReportInput) {
			in.Kind, in.NativePayload = CrashNative, json.RawMessage(`[1,2]`)
		},
		"native payload over 64 KiB": func(in *ClientCrashReportInput) {
			in.Kind = CrashNative
			in.NativePayload = json.RawMessage(`{"a":"` + strings.Repeat("p", 64<<10) + `"}`)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			in := validCrash()
			in.Breadcrumbs = append([]ClientCrashBreadcrumb(nil), in.Breadcrumbs...)
			mutate(&in)
			memory := &crashMemory{}
			_, err := NewClientCrashes(memory, nil).Report(t.Context(), ClientCrashReporter{UserID: "u"}, in)
			if !errors.Is(err, ErrInvalidClientCrash) {
				t.Fatalf("Report error = %v, want ErrInvalidClientCrash", err)
			}
			if len(memory.records) != 0 {
				t.Fatal("a rejected report was stored")
			}
		})
	}
}

func TestClientCrashBodyRefusesUnknownFields(t *testing.T) {
	t.Parallel()
	var in ClientCrashReportInput
	err := json.Unmarshal([]byte(`{"kind":"native","userId":"u-admin"}`), &in)
	if err == nil {
		t.Fatal("a body naming its own user decoded; the server must own who sent a report")
	}
	err = json.Unmarshal([]byte(`{"breadcrumbs":[{"at":"x","category":"http","message":"m","headers":{}}]}`), &in)
	if err == nil {
		t.Fatal("a breadcrumb smuggling headers decoded")
	}
}

func TestScrubCrashTextKeepsNoCredentialOrQuery(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"GET https://loomarr.example/v1/channels/7/play.m3u8?token=s3cret#t=4 failed": "GET https://loomarr.example/v1/channels/7/play.m3u8 failed",
		"fetch /v1/channels/7/play.m3u8?sig=s3cret&exp=1 → 403":                       "fetch /v1/channels/7/play.m3u8 → 403",
		// The removal runs to the next space: a bracket may sit inside a signature.
		"at load (http://10.0.0.2:8081/index.bundle?platform=ios&dev=true:12:4)": "at load (http://10.0.0.2:8081/index.bundle",
		"ws://loomarr.example?session=s3cret closed":                             "ws://loomarr.example closed",
		"https://user:s3cret@loomarr.example/v1/me":                              "https://[redacted]@loomarr.example/v1/me",
		"Error loading playlist.m3u8?s=s3cret&e=123":                             "Error loading playlist.m3u8",
		"hls fragLoadError seg00012.ts?sig=ch1:1760000000:s3cret":                "hls fragLoadError seg00012.ts",
		"Why did this fail? Nobody knows":                                        "Why did this fail? Nobody knows",
		"crash at\x00 boot":                                                      "crash at boot",
	}
	for in, want := range cases {
		if got := scrubCrashText(in, maxCrashMessageBytes); got != want {
			t.Errorf("scrubCrashText(%q)\n got %q\nwant %q", in, got, want)
		}
	}
	for _, leaky := range []string{
		"request headers:\nAuthorization: Bearer s3cret\nAccept: */*",
		`{"authorization":"Bearer s3cret"}`,
		`{"Authorization":"Basic s3cret"}`,
		`{"Cookie":"loomarr_session=s3cret; other=s3cret"}`,
		`{"token":"s3cret"}`,
		`{"access_token":"s3cret","password":"s3cret"}`,
		`headers: {'X-Api-Key': 's3cret'}`,
		"[Authorization: Basic s3cret]",
		"Authorization=Basic s3cret",
		`document.cookie = "a=1; loomarr_session=s3cret"`,
		"Cookie: loomarr_session=s3cret; theme=s3cret",
		"set-cookie: loomarr_session=s3cret; Path=/; HttpOnly",
		"refresh failed token=s3cret",
		"api_key: s3cret",
		"x-device-token: s3cret",
		"https://h/p?a=b)c=s3cret",
		"sessionId=s3cret",
		`{"sessionId":"s3cret","tokenId":"s3cret"}`,
		"https://h:8080/v1/x%3Ftoken%3Ds3cret",
		"bearer: s3cret",
		"Bearer s3cret",
	} {
		if got := scrubCrashText(leaky, maxCrashMessageBytes); strings.Contains(got, "s3cret") {
			t.Errorf("scrubCrashText(%q) = %q, still carries the secret", leaky, got)
		}
	}
	// Ordinary stack frames naming tokens or sessions are not credentials and survive whole.
	for _, frame := range []string{
		"at refreshToken (index.android.bundle:1:2345)",
		"at Object.getSession (App.tsx:12:3)",
		"(evaluating 'a.token.x')",
	} {
		if got := scrubCrashText(frame, maxCrashMessageBytes); got != frame {
			t.Errorf("scrubCrashText(%q) = %q, want the frame unchanged", frame, got)
		}
	}
}

func TestClientCrashStoresScrubbedReportWithServerIdentity(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	memory := &crashMemory{}
	crashes := NewClientCrashes(memory, func() time.Time { return now })
	in := validCrash()
	in.Kind = CrashNative
	in.Message = "EXC_BAD_ACCESS loading https://loomarr.example/v1/live/7.m3u8?token=s3cret"
	in.Breadcrumbs = []ClientCrashBreadcrumb{{At: "2026-10-13T07:59:58+02:00", Category: BreadcrumbHTTP,
		Message: "GET /v1/live/7.m3u8?token=s3cret 200"}}
	in.NativePayload = json.RawMessage(`{"callStackTree":{"frames":[{"binaryName":"Loomarr"}]},` +
		`"url":"https://loomarr.example/v1/live/7.m3u8?token=s3cret","token":"s3cret","n":3}`)

	id, err := crashes.Report(t.Context(), ClientCrashReporter{UserID: "u-kid", DeviceID: "hash-1", DeviceName: "Phone"}, in)
	if err != nil {
		t.Fatal(err)
	}
	report, err := crashes.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), "s3cret") {
		t.Fatalf("stored report still carries the secret: %s", encoded)
	}
	if !json.Valid(report.NativePayload) || !strings.Contains(string(report.NativePayload), `"binaryName":"Loomarr"`) {
		t.Fatalf("native payload = %s, want valid scrubbed JSON keeping its frames", report.NativePayload)
	}
	if report.UserID != "u-kid" || report.DeviceID != "hash-1" || report.DeviceName != "Phone" {
		t.Fatalf("reporter = %q/%q/%q, want the server-derived identity", report.UserID, report.DeviceID, report.DeviceName)
	}
	if report.ReceivedAt != "2026-10-13T09:00:00Z" || report.Breadcrumbs[0].At != "2026-10-13T05:59:58Z" {
		t.Fatalf("times = received %s, breadcrumb %s; want server clock and UTC", report.ReceivedAt, report.Breadcrumbs[0].At)
	}
	if _, err := crashes.Get(t.Context(), "crash_missing"); !errors.Is(err, ErrClientCrashNotFound) {
		t.Fatalf("missing report error = %v, want ErrClientCrashNotFound", err)
	}
}

// One crash-looping device must not lock its owner's other devices out of reporting.
func TestClientCrashRateLimitIsPerDevice(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	crashes := NewClientCrashes(&crashMemory{}, func() time.Time { return now })
	phone := ClientCrashReporter{UserID: "u-kid", DeviceID: "phone"}
	for i := range crashReportBurst {
		if _, err := crashes.Report(t.Context(), phone, validCrash()); err != nil {
			t.Fatalf("report %d: %v", i, err)
		}
	}
	if _, err := crashes.Report(t.Context(), phone, validCrash()); !errors.Is(err, ErrClientCrashRateLimited) {
		t.Fatalf("report past the burst = %v, want ErrClientCrashRateLimited", err)
	}
	tv := ClientCrashReporter{UserID: "u-kid", DeviceID: "tv"}
	if _, err := crashes.Report(t.Context(), tv, validCrash()); err != nil {
		t.Fatalf("a second device of the same person was limited: %v", err)
	}
	now = now.Add(time.Minute)
	if _, err := crashes.Report(t.Context(), phone, validCrash()); err != nil {
		t.Fatalf("the phone did not recover after a minute: %v", err)
	}
}

// One person's devices together cannot fill the retained cap and evict everyone else's reports;
// a refused report is charged to neither bucket.
func TestClientCrashRateLimitCapsAPersonAcrossDevices(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	crashes := NewClientCrashes(&crashMemory{}, func() time.Time { return now })
	accepted := 0
	for device := range 10 {
		for range crashReportBurst {
			from := ClientCrashReporter{UserID: "u-kid", DeviceID: string(rune('a' + device))}
			if _, err := crashes.Report(t.Context(), from, validCrash()); err == nil {
				accepted++
			}
		}
	}
	if accepted != crashReportPersonBurst {
		t.Fatalf("accepted %d across ten devices, want the person's burst of %d", accepted, crashReportPersonBurst)
	}
	if _, err := crashes.Report(t.Context(), ClientCrashReporter{UserID: "u-other", DeviceID: "z"}, validCrash()); err != nil {
		t.Fatalf("another person was limited: %v", err)
	}
	// Device "j" was refused by the person bucket every time, so its own burst is untouched.
	now = now.Add(time.Hour)
	for i := range crashReportBurst {
		if _, err := crashes.Report(t.Context(), ClientCrashReporter{UserID: "u-kid", DeviceID: "j"}, validCrash()); err != nil {
			t.Fatalf("device j report %d after the person bucket refilled: %v", i, err)
		}
	}
}

func TestClientCrashLabelsAreScrubbedAndNativePayloadIsNotHTMLEscaped(t *testing.T) {
	t.Parallel()
	memory := &crashMemory{}
	in := validCrash()
	in.Kind, in.AppVersion = CrashNative, "1.4.0 token=s3cret"
	in.NativePayload = json.RawMessage(`{"exceptionReason":"a < b && c > d","X-Device-Token":"s3cret"}`)
	if _, err := NewClientCrashes(memory, nil).Report(t.Context(), ClientCrashReporter{}, in); err != nil {
		t.Fatal(err)
	}
	stored := memory.records[0]
	if strings.Contains(stored.AppVersion, "s3cret") || strings.Contains(stored.NativePayload, "s3cret") {
		t.Fatalf("stored labels/payload carry the secret: %q %s", stored.AppVersion, stored.NativePayload)
	}
	if !strings.Contains(stored.NativePayload, `"a < b && c > d"`) {
		t.Fatalf("native payload was HTML-escaped: %s", stored.NativePayload)
	}
}

func TestClientCrashListPagesNewestFirst(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	crashes := NewClientCrashes(&crashMemory{}, func() time.Time { return now })
	var ids []string
	for i := range 5 {
		id, err := crashes.Report(t.Context(), ClientCrashReporter{DeviceID: string(rune('a' + i))}, validCrash())
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
		now = now.Add(time.Second)
	}
	var seen []string
	cursor := ""
	for range 3 {
		page, err := crashes.List(t.Context(), 2, cursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			seen = append(seen, item.ID)
		}
		if cursor = page.NextCursor; cursor == "" {
			break
		}
	}
	want := []string{ids[4], ids[3], ids[2], ids[1], ids[0]}
	if strings.Join(seen, ",") != strings.Join(want, ",") {
		t.Fatalf("pages = %v, want newest first %v", seen, want)
	}
	for _, bad := range []string{"not-base64!", "e30"} {
		if _, err := crashes.List(t.Context(), 2, bad); !errors.Is(err, ErrInvalidClientCrashQuery) {
			t.Fatalf("cursor %q error = %v, want ErrInvalidClientCrashQuery", bad, err)
		}
	}
}
