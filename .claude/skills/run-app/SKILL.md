---
name: run-app
description: Launch Loomarr locally (Air backend + Vite frontend) and drive it in a real browser against the maintainer's live stack. Use when asked to run/start the app, screenshot a surface, or confirm a change works in the real UI rather than only in tests.
---

# Running Loomarr

Two processes. The backend serves the API on **:8080**; Vite serves the frontend on
**:5173** with HMR.

⚠ **Always look at :5173, never :8080.** The backend also serves an *embedded* SPA build
that is almost always stale — a change that is live on 5173 is invisible on 8080, which
reads as "my change didn't work" and costs an hour.

## 1. Backend

```bash
LOOMARR_DEV_LOGIN=1 make dev-be > /tmp/loomarr-be.log 2>&1 &
until curl -sf http://localhost:8080/v1/healthz >/dev/null 2>&1; do sleep 2; done
```

`make dev-be` runs Air (via `go run`, so it is never in `go.mod`), which sources `.env` and
rebuilds on any Go change. It talks to the **real** store at `./data/loomarr.db` and the
maintainer's real Emby/Tunarr/Ollama over Tailscale.

`LOOMARR_DEV_LOGIN=1` is what makes browser automation possible — see §3. Without it
`POST /v1/auth/dev-login` **404s** (fail-closed by design, not a bug).

**Confirm you are talking to the process you just built** — this repo has lost a session to
an orphaned `go run` binary serving hours-old code:

```bash
curl -s http://localhost:8080/v1/system/version | python3 -m json.tool | head -12
```

`commit` should match `git rev-parse --short HEAD` and `startedAt` should be seconds old. If
either disagrees, kill the stale process first:

```bash
pkill -f 'tmp/loomarr-dev'; pkill -f 'air@v1.67.3'
```

## 2. Frontend

```bash
cd web/apps/web && npx pnpm@11.13.1 dev > /tmp/loomarr-fe.log 2>&1 &
until curl -sf http://localhost:5173 >/dev/null 2>&1; do sleep 2; done
```

There is **no `make dev-fe`** — this is the command.

## 3. Driving it in a browser

`chromium-cli` is not installed; use the repo's own Playwright. The browser binary is **not**
on the host either (the visual suite runs in Docker), so once per machine:

```bash
npx playwright install chromium
```

⚠ **The driver script must live inside `web/apps/web/`**, not `/tmp` — a script outside the
pnpm workspace cannot resolve `@playwright/test` and dies with `ERR_MODULE_NOT_FOUND`.

⚠ **A `Bearer $API_TOKEN` header does NOT authenticate the SPA.** It works for `curl` against
the API, but the frontend uses a session cookie, so a bearer-only browser lands on
`/login?redirect=…`. Get a real cookie from the dev-login route:

```js
import { chromium } from '@playwright/test';
const b = await chromium.launch();
const ctx = await b.newContext({ viewport: { width: 1280, height: 1400 } });
const p = await ctx.newPage();
p.on('console', m => { if (m.type() === 'error') console.log('PAGE ERROR:', m.text()); });

await ctx.request.post('http://localhost:8080/v1/auth/dev-login'); // session cookie

await p.goto('http://localhost:5173/channels/<id>', { waitUntil: 'networkidle' });
await p.getByRole('button', { name: 'Programming', exact: true }).click();
await p.waitForTimeout(3000);
await p.screenshot({ path: '/tmp/shot.png', fullPage: true });
await b.close();
```

Run it, then **delete it** (`web/apps/web` is linted; a stray `.mjs` fails `make fe`).

### Selector notes that cost time

- **Channel sub-tabs are `<button>`, not links or ARIA tabs.** `getByRole('tab')` and
  `getByRole('link', {name:'Programming'})` both find nothing; use
  `getByRole('button', { name: 'Programming', exact: true })`.
- A `<legend class="sr-only">` duplicating a visible `<Label>` makes `getByText` fail
  strict mode with two matches. Target `getByRole('group', { name })` instead.
- One `Encountered two children with the same key` console error is pre-existing on the
  channel page. Filter it out rather than chasing it.

## 4. Then actually look

Take the screenshot **and read it**. This repo's own history is the argument: a wrapped
timestamp made every Dashboard row uneven with the whole suite green, and it was caught only
by opening the image. A green run is not a look.

## Useful state

```bash
set -a; . ./.env; set +a
curl -s -H "Authorization: Bearer $API_TOKEN" http://localhost:8080/v1/channels | python3 -m json.tool | head
```

`API_TOKEN` is the break-glass admin token for API calls (not for the SPA — see §3).
Sourcing `.env` in one shell and curling in another does not carry the variable; do both in
the same `Bash` call.
