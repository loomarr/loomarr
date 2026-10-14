# Channel writers: live evidence for #1899

The setup was an isolated lane backend from this worktree (`make dev-be`, its own SQLite) and the
demo library (`make demo-library` + `make demo-seed`, with invented titles). The page was served by
the Vite frontend (`make dev-fe`). The filler jobs, `channel-maintenance` and `channel-recurate`
were paused first. The browser was Chromium at 1440×1000, signed in as the lane's dev admin.

Each run opened a seeded channel's Programming tab and made four edits with no pause between them:

1. typed Era From;
2. clicked To, so the blur saved From;
3. typed To and tabbed out, so the blur saved To;
4. clicked Remove on the last lineup title, then renamed the channel in the header and pressed
   Enter.

After four seconds the run took a screenshot, reloaded the page, took a second screenshot and read
the channel back from the API.

On the bare lane every PATCH returned before the next action, so even `main` saved everything
there: two runs per side passed. So the runs below add **300 ms of round-trip latency**, through
Chrome DevTools network emulation. A home network can have that, and it puts each edit inside the
previous edit's save.

| Run | Channel | PATCHes (fields → status) | Saved after reload |
| --- | --- | --- | --- |
| Before (`origin/main` frontend) | Saturday Morning Cartoons | policy → 200, lineup → **409**, policy → 200, name → **409** | Era 1960–1969; the removed title **still there**; the name **unchanged** |
| After (this PR) | Premium 4K | policy → 200, policy + lineup + name → 200 | Era 1990–1999; title removed; renamed "Premium 4K Prime" |

The API read agreed with the reloaded page in both runs. Without added latency, two more
after-fix runs (Mystery Hour, Sunday Matinee) sent four PATCHes each, all 200, and every edit was
saved.

| | Typed | After reload |
| --- | --- | --- |
| Before | [before-typed.png](channel-writers-1899-2026-10-13/before-typed.png) | [before-reloaded.png](channel-writers-1899-2026-10-13/before-reloaded.png) |
| After | [after-latency-typed.png](channel-writers-1899-2026-10-13/after-latency-typed.png) | [after-latency-reloaded.png](channel-writers-1899-2026-10-13/after-latency-reloaded.png) |

## Remove, below @md

The same lane at a 390×844 viewport, on Saturday Morning Cartoons' Programming tab:
[after-390.png](channel-writers-1899-2026-10-13/after-390.png). Remove now sits last in each row, on
its own line at the right, with a 44 px target. Tab from a row's grip visits the grip, the first
season, the last season, Keep, Less like this, Never, Surprise me, and then Remove. That is the
order they appear on screen. The next Tab goes to the following row's grip.
