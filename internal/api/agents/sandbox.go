package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"modernc.org/quickjs"
)

// The Code Mode sandbox (#4486): an admin's agent writes JavaScript, and it runs here with ONE way
// out, loomarr.request, which the caller turns into an in-process API call as that admin.
//
// THE ENGINE IS QUICKJS, TRANSPILED TO GO (modernc.org/quickjs): no cgo, no Wasm, no process. It is
// a library inside the server like the SQLite driver beside it, and the script is the agent's
// input, never Loomarr code (AGENTS.md rule 4). It was picked over goja because only it enforces a
// memory limit itself, at the allocator, so one `"x".repeat(2**30)` is refused instead of
// allocated (the spike on #4486 has the numbers).
//
// What the script has: the language and its built-ins, `loomarr.request`, and `console.log`. What
// it lacks: modules (no loader is installed, so `import` fails), files, sockets, timers and
// anything else from the host. Each run gets a fresh VM, so nothing carries between runs.

// sandboxLimits bound one run. Hitting any of them ends the run with an error that says which.
type sandboxLimits struct {
	Time     time.Duration // wall clock for the whole run, requests included
	Memory   uintptr       // the VM's heap
	Requests int           // loomarr.request calls
	Output   int           // bytes of result plus console output
	Code     int           // bytes of script
}

var defaultSandboxLimits = sandboxLimits{
	Time:     20 * time.Second,
	Memory:   64 << 20,
	Requests: 50,
	Output:   64 << 10,
	Code:     32 << 10,
}

const (
	// maxScriptPath and maxScriptBody bound one loomarr.request's arguments. They are copied out of
	// the VM into Go, where its memory limit does not reach.
	maxScriptPath = 2 << 10
	maxScriptBody = 1 << 20
	// maxRecordedPath is how much of a path a run records.
	maxRecordedPath = 200
)

// recordedPath is a request's path as a run records it: without its query, which may carry values
// that do not belong in an audit log, and capped.
func recordedPath(path string) string {
	p, _, _ := strings.Cut(path, "?")
	if short := truncateRunes(p, maxRecordedPath); short != p {
		return short + "…"
	}
	return p
}

// sandboxRequest is one loomarr.request call, as the run's audit records it: never the body.
type sandboxRequest struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Status int    `json:"status"`
}

// sandboxReply is what a host request answers the script.
type sandboxReply struct {
	Status int `json:"status"`
	Body   any `json:"body"`
}

// requestFunc performs one loomarr.request. body is the script's argument as JSON, or nil.
type requestFunc func(ctx context.Context, method, path string, body json.RawMessage) (sandboxReply, error)

// sandboxRun is a finished run: what the script returned, what it logged, and the requests it made.
type sandboxRun struct {
	Result   json.RawMessage
	Logs     []string
	Requests []sandboxRequest
}

var (
	errSandboxTime     = errors.New("the script ran past its time limit")
	errSandboxMemory   = errors.New("the script ran out of memory")
	errSandboxRequests = errors.New("the script made too many loomarr.request calls")
	errSandboxOutput   = errors.New("the script's result and logs are too large")
)

// The prelude runs before the script. It wraps the two host functions in the API the script sees
// and removes the raw ones from the global object, so the script cannot call them with arguments
// the wrappers would not pass.
const sandboxPrelude = `
(() => {
  const send = globalThis.__loomarr_request;
  const log = globalThis.__loomarr_log;
  delete globalThis.__loomarr_request;
  delete globalThis.__loomarr_log;
  const text = (v) => typeof v === "string" ? v : JSON.stringify(v);
  globalThis.loomarr = Object.freeze({
    request(method, path, body) {
      const r = JSON.parse(send(String(method), String(path), body === undefined ? "" : JSON.stringify(body)));
      if (r.error) throw new Error(r.error);
      return r.reply;
    },
  });
  globalThis.console = Object.freeze({ log: (...a) => log(a.map(text).join(" ")) });
})();
`

// runScript runs code as the body of an async function, so it may `await` and `return`.
func runScript(ctx context.Context, code string, lim sandboxLimits, request requestFunc) (sandboxRun, error) {
	var run sandboxRun
	if len(code) > lim.Code {
		return run, fmt.Errorf("the script is %d bytes; the limit is %d", len(code), lim.Code)
	}
	ctx, cancel := context.WithTimeout(ctx, lim.Time)
	defer cancel()

	vm, err := quickjs.NewVM()
	if err != nil {
		return run, err
	}
	defer func() { _ = vm.Close() }()
	vm.SetMemoryLimit(lim.Memory)
	stop := context.AfterFunc(ctx, vm.Interrupt)
	defer stop()

	// A host call's failure is handed back to the script as {error}, which the prelude throws, so
	// the script can catch it; a limit additionally interrupts the VM, so catching does not help.
	// Host functions run on this goroutine, inside Eval, so these need no lock.
	output := 0
	var limitHit error
	hit := func(e error) {
		if limitHit == nil {
			limitHit = e
		}
		vm.Interrupt()
	}
	answer := func(reply *sandboxReply, msg string) string {
		raw, _ := json.Marshal(struct {
			Reply *sandboxReply `json:"reply,omitempty"`
			Error string        `json:"error,omitempty"`
		}{reply, msg})
		return string(raw)
	}
	if err := vm.RegisterFunc("__loomarr_request", func(method, path, body string) string {
		if len(run.Requests) >= lim.Requests {
			hit(errSandboxRequests)
			return answer(nil, errSandboxRequests.Error())
		}
		// ⚠ Everything a host call keeps lives OUTSIDE the VM's memory limit, so what it keeps is
		// bounded here: the method and path are capped before they are recorded, and a body is
		// refused rather than copied on.
		method = strings.ToUpper(method)
		if len(method) > 16 {
			return answer(nil, "method is not an HTTP method")
		}
		shown := recordedPath(path)
		if len(path) > maxScriptPath || len(body) > maxScriptBody {
			run.Requests = append(run.Requests, sandboxRequest{Method: method, Path: shown})
			return answer(nil, fmt.Sprintf("a request's path may be %d bytes and its body %d", maxScriptPath, maxScriptBody))
		}
		var raw json.RawMessage
		if body != "" {
			raw = json.RawMessage(body)
		}
		reply, err := request(ctx, method, path, raw)
		run.Requests = append(run.Requests, sandboxRequest{Method: method, Path: shown, Status: reply.Status})
		if err != nil {
			return answer(nil, err.Error())
		}
		return answer(&reply, "")
	}, false); err != nil {
		return run, err
	}
	if err := vm.RegisterFunc("__loomarr_log", func(line string) {
		if output += len(line) + 1; output > lim.Output {
			hit(errSandboxOutput)
			return
		}
		run.Logs = append(run.Logs, line)
	}, false); err != nil {
		return run, err
	}
	if _, err := vm.Eval(sandboxPrelude, quickjs.EvalGlobal); err != nil {
		return run, fmt.Errorf("sandbox prelude: %w", err)
	}

	// The script's outcome lands in two globals the host reads back; JSON.stringify happens inside
	// the VM so the result is plain data, whatever the script returned.
	const wrapper = `globalThis.__done = false;
(async () => { %s
})().then(
  (v) => { globalThis.__result = v === undefined ? "null" : JSON.stringify(v); globalThis.__done = true; },
  (e) => { globalThis.__error = String(e && e.stack ? e + "\n" + e.stack : e); globalThis.__done = true; },
);`
	_, evalErr := vm.Eval(fmt.Sprintf(wrapper, code), quickjs.EvalGlobal)
	if evalErr == nil {
		_, evalErr = vm.ExecutePendingJobs()
	}
	if limitHit != nil {
		return run, limitHit
	}
	if ctx.Err() != nil {
		return run, errSandboxTime
	}
	if evalErr != nil {
		return run, classifyVMError(evalErr)
	}
	if done, _ := vm.Eval("globalThis.__done", quickjs.EvalGlobal); done != true {
		return run, errors.New("the script is still waiting on something that never finishes")
	}
	if msg, _ := vm.Eval("globalThis.__error", quickjs.EvalGlobal); msg != nil {
		if s, ok := msg.(string); ok {
			return run, classifyVMError(errors.New(s))
		}
	}
	res, _ := vm.Eval("globalThis.__result", quickjs.EvalGlobal)
	s, _ := res.(string)
	if output+len(s) > lim.Output {
		return run, errSandboxOutput
	}
	run.Result = json.RawMessage(s)
	return run, nil
}

// classifyVMError names the limit behind an engine error, so the agent is told what to change.
func classifyVMError(err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "out of memory"):
		return errSandboxMemory
	case strings.Contains(msg, "interrupted"):
		return errSandboxTime
	}
	return fmt.Errorf("the script failed: %s", untrusted(msg))
}
