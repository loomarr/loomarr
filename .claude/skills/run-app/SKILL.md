---
name: run-app
description: Launch Loomarr locally (Air backend + Vite frontend) and drive it in a real browser against the maintainer's live stack. Use when asked to run/start the app, screenshot a surface, or confirm a change works in the real UI rather than only in tests.
---

# Running Loomarr

Two processes: the backend (Air) serves the API, and Vite serves the frontend with HMR. Each
worktree has its own ports. The primary checkout uses **:8080** and **:5173**, and a secondary
worktree gets its own pair. Read them first, in the same `Bash` call as anything that uses them:

```bash
eval "$(./scripts/dev-env.sh export)"; BE=$LOOMARR_DEV_PORT; FE=$LOOMARR_FE_PORT; mkdir -p .artifacts
```

`make agent-env` prints the same values, plus this worktree's database and dev-login setting.

⚠ **Always look at the frontend port, never the backend's.** The backend also serves an
*embedded* SPA build that is almost always stale — a change that is live in Vite is invisible
on the backend port, which reads as "my change didn't work" and costs an hour.

## 1. Backend

```bash
LOOMARR_DEV_LOGIN=1 make dev-be > .artifacts/dev-be.log 2>&1 &
until curl -sf "http://localhost:$BE/v1/healthz" >/dev/null 2>&1; do sleep 2; done
```

`make dev-be` runs Air (via `go run`, so it is never in `go.mod`), which sources `.env` and
rebuilds on any Go change. In the primary checkout it talks to the **real** store at
`./data/loomarr.db` and the maintainer's real Emby/Tunarr/Ollama over Tailscale. A secondary
worktree has its own database and an automatic dev login.

`LOOMARR_DEV_LOGIN=1` is what makes browser automation possible in the primary checkout — see
§3. Without it `POST /v1/auth/dev-login` **404s** (fail-closed by design, not a bug).

`make dev-be` stops a server a removed worktree left on this port, refuses to start beside one
this worktree already runs, and builds before it serves anything. If the tree does not compile,
nothing answers: read the log rather than waiting on the health check.

**Confirm you are talking to the process you just built:**

```bash
curl -s "http://localhost:$BE/v1/system/version" | python3 -m json.tool | head -12
```

`commit` should match `git rev-parse --short HEAD` and `startedAt` should be seconds old. If
either disagrees, replace this worktree's server. Never `pkill` by command line: it matches
every worktree's Air.

```bash
DEV_BE_REPLACE=1 LOOMARR_DEV_LOGIN=1 make dev-be > .artifacts/dev-be.log 2>&1 &
```

## 2. Frontend

```bash
make dev-fe > .artifacts/dev-fe.log 2>&1 &
until curl -sf "http://localhost:$FE" >/dev/null 2>&1; do sleep 2; done
```

`make dev-fe` installs dependencies and regenerates the API client and tokens when a pull or
branch switch changed them, and stops a server a removed worktree left on this port.

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

const be = process.env.BE ?? '8080', fe = process.env.FE ?? '5173'; // pass BE=$BE FE=$FE
await ctx.request.post(`http://localhost:${be}/v1/auth/dev-login`); // session cookie

await p.goto(`http://localhost:${fe}/channels/<id>`, { waitUntil: 'networkidle' });
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
set -a; . ./.env; set +a; eval "$(./scripts/dev-env.sh export)"
curl -s -H "Authorization: Bearer $API_TOKEN" "http://localhost:$LOOMARR_DEV_PORT/v1/channels" | python3 -m json.tool | head
```

`API_TOKEN` is the break-glass admin token for API calls (not for the SPA — see §3).
Sourcing `.env` in one shell and curling in another does not carry the variable; do both in
the same `Bash` call.
