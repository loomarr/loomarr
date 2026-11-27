package agents

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeAPI answers loomarr.request from a table, and 403s anything else, the way a role-checked
// route refuses.
func fakeAPI(routes map[string]any) requestFunc {
	return func(_ context.Context, method, path string, _ json.RawMessage) (sandboxReply, error) {
		if body, ok := routes[method+" "+path]; ok {
			return sandboxReply{Status: 200, Body: body}, nil
		}
		return sandboxReply{Status: 403, Body: map[string]any{"title": "Forbidden"}}, nil
	}
}

func testLimits() sandboxLimits {
	l := defaultSandboxLimits
	l.Time = 2 * time.Second
	return l
}

func TestScriptJoinsThreeReadsInOneRun(t *testing.T) {
	api := fakeAPI(map[string]any{
		"GET /v1/channels":       []map[string]any{{"id": "c1", "name": "Cartoons"}, {"id": "c2", "name": "Films"}},
		"GET /v1/playout/status": map[string]any{"streams": []map[string]any{{"channelId": "c2", "state": "live"}}},
		"GET /v1/household":      map[string]any{"timezone": "UTC"},
	})
	code := `
const [channels, playout, home] = await Promise.all([
  loomarr.request("GET", "/v1/channels"),
  loomarr.request("get", "/v1/playout/status"),
  loomarr.request("GET", "/v1/household"),
]);
const live = new Set(playout.body.streams.filter(s => s.state === "live").map(s => s.channelId));
console.log("zone", home.body.timezone);
return channels.body.map(({ id, name }) => ({ name, live: live.has(id) }));`
	run, err := runScript(context.Background(), code, testLimits(), api)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(run.Result); got != `[{"name":"Cartoons","live":false},{"name":"Films","live":true}]` {
		t.Fatalf("result = %s", got)
	}
	if len(run.Requests) != 3 || run.Requests[1].Method != "GET" || run.Logs[0] != "zone UTC" {
		t.Fatalf("requests = %+v, logs = %v", run.Requests, run.Logs)
	}
}

func TestScriptSeesTheRefusalTheRouteGives(t *testing.T) {
	run, err := runScript(context.Background(), `return (await loomarr.request("DELETE", "/v1/users/u1")).status`, testLimits(), fakeAPI(nil))
	if err != nil || string(run.Result) != "403" || run.Requests[0].Status != 403 {
		t.Fatalf("run = %+v, %v; want the 403 handed to the script and audited", run, err)
	}
}

func TestEndlessScriptIsStopped(t *testing.T) {
	l := testLimits()
	l.Time = 300 * time.Millisecond
	start := time.Now()
	_, err := runScript(context.Background(), `while (true) {}`, l, fakeAPI(nil))
	if !errors.Is(err, errSandboxTime) || time.Since(start) > 2*time.Second {
		t.Fatalf("err = %v after %v, want the time limit near 300ms", err, time.Since(start))
	}
	// An endless loop inside a promise chain is stopped the same way.
	_, err = runScript(context.Background(), `await null; for (;;) {}`, l, fakeAPI(nil))
	if !errors.Is(err, errSandboxTime) {
		t.Fatalf("async loop err = %v, want the time limit", err)
	}
}

func TestAllocatingScriptIsStopped(t *testing.T) {
	for name, code := range map[string]string{
		"growing": `const a = []; for (;;) a.push("x".repeat(1 << 20) + a.length);`,
		// 2**29 bytes: under QuickJS's string-length cap, so the allocator's limit is what refuses it.
		"one shot": `return "x".repeat(2 ** 29).length;`,
		"buffer":   `return new ArrayBuffer(2 ** 30).byteLength;`,
	} {
		t.Run(name, func(t *testing.T) {
			// A generous clock, so the memory limit is what stops it even under -race, where the
			// transpiled engine runs many times slower.
			l := testLimits()
			l.Time = time.Minute
			_, err := runScript(context.Background(), code, l, fakeAPI(nil))
			if !errors.Is(err, errSandboxMemory) {
				t.Fatalf("err = %v, want the memory limit", err)
			}
		})
	}
}

// Catching the limit's error does not keep the flood going: the VM is interrupted as well.
func TestRequestFloodIsStopped(t *testing.T) {
	code := `for (;;) { try { await loomarr.request("GET", "/v1/channels"); } catch (e) {} }`
	run, err := runScript(context.Background(), code, testLimits(), fakeAPI(map[string]any{"GET /v1/channels": []any{}}))
	if !errors.Is(err, errSandboxRequests) || len(run.Requests) != defaultSandboxLimits.Requests {
		t.Fatalf("err = %v after %d requests, want the request limit at %d", err, len(run.Requests), defaultSandboxLimits.Requests)
	}
}

// What a host call keeps lives outside the VM's memory limit, so a huge path or body is refused
// before it is kept: fifty 30 MiB paths would otherwise hold 1.5 GiB in Go.
func TestHugeRequestArgumentsAreRefusedNotKept(t *testing.T) {
	calls := 0
	api := func(context.Context, string, string, json.RawMessage) (sandboxReply, error) {
		calls++
		return sandboxReply{Status: 200}, nil
	}
	code := `
const big = "a".repeat(30 << 20);
const out = [];
for (let i = 0; i < 3; i++) { try { loomarr.request("GET", "/v1/" + big); } catch (e) { out.push(String(e)); } }
try { loomarr.request("POST", "/v1/x", { blob: "b".repeat(2 << 20) }); } catch (e) { out.push(String(e)); }
return out.length;`
	l := testLimits()
	l.Time = time.Minute
	run, err := runScript(context.Background(), code, l, api)
	if err != nil || string(run.Result) != "4" || calls != 0 {
		t.Fatalf("run = %s, err = %v, calls = %d; want all four refused before the API", run.Result, err, calls)
	}
	for _, r := range run.Requests {
		if len(r.Path) > maxRecordedPath+len("…") {
			t.Fatalf("a refused request kept a %d-byte path", len(r.Path))
		}
	}
}

func TestOutputIsCapped(t *testing.T) {
	if _, err := runScript(context.Background(), `return "x".repeat(1 << 20)`, testLimits(), fakeAPI(nil)); !errors.Is(err, errSandboxOutput) {
		t.Fatalf("large result err = %v, want the output limit", err)
	}
	if _, err := runScript(context.Background(), `for (;;) console.log("y".repeat(1000))`, testLimits(), fakeAPI(nil)); !errors.Is(err, errSandboxOutput) {
		t.Fatalf("log flood err = %v, want the output limit", err)
	}
}

func TestScriptHasNoOtherWayOut(t *testing.T) {
	for name, code := range map[string]string{
		"import":          `await import("os"); return "escaped";`,
		"require":         `require("fs"); return "escaped";`,
		"fetch":           `await fetch("http://example.invalid"); return "escaped";`,
		"raw host fn":     `__loomarr_request("GET", "/v1/channels", ""); return "escaped";`,
		"std":             `std.loadFile("/etc/hostname"); return "escaped";`,
		"replace loomarr": `loomarr.request = () => 1; return loomarr.request() === 1 ? "escaped" : "frozen";`,
	} {
		t.Run(name, func(t *testing.T) {
			run, err := runScript(context.Background(), code, testLimits(), fakeAPI(nil))
			if strings.Contains(string(run.Result), "escaped") {
				t.Fatalf("%s worked: %s", name, run.Result)
			}
			if err == nil && string(run.Result) != `"frozen"` {
				t.Fatalf("%s: no error and result %s", name, run.Result)
			}
		})
	}
}

func TestOversizedScriptIsRefusedUnrun(t *testing.T) {
	calls := 0
	api := func(context.Context, string, string, json.RawMessage) (sandboxReply, error) {
		calls++
		return sandboxReply{}, nil
	}
	code := `await loomarr.request("GET", "/x");` + strings.Repeat(" ", defaultSandboxLimits.Code)
	if _, err := runScript(context.Background(), code, testLimits(), api); err == nil || calls != 0 {
		t.Fatalf("err = %v, calls = %d; want a refusal before running", err, calls)
	}
}

func TestScriptErrorIsReported(t *testing.T) {
	_, err := runScript(context.Background(), `throw new Error("nope")`, testLimits(), fakeAPI(nil))
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("err = %v, want the script's own error", err)
	}
	_, err = runScript(context.Background(), `return (`, testLimits(), fakeAPI(nil))
	if err == nil || !strings.Contains(err.Error(), "SyntaxError") {
		t.Fatalf("err = %v, want a syntax error", err)
	}
}
