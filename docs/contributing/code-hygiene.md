# Code hygiene: unused code

**For:** contributors removing code, or told by CI that something is unused.
**You'll get:** the two tools that find dead code, and which one blocks a merge.

Two tools find code nothing uses. One is a gate; the other is a report.

| Tool | Scope | Run it | In CI |
| --- | --- | --- | --- |
| knip | `web/`: files, exports and dependencies | `make knip` | Gate: `make fe` fails on any finding |
| deadcode | Go: functions no `./cmd` binary reaches | `make deadcode` | Report: `deadcode.yml`, monthly |

## knip (frontend gate)

`make knip` regenerates the route tree and API client, then runs knip with `web/knip.ts`. A
pull request that adds an unused file, export or dependency fails the frontend job.

When knip reports something:

1. **Delete it.** Most findings are code that lost its last caller: a barrel nobody imports, a
   constant exported "for later", a dependency a refactor stopped using.
2. **Stop exporting it** when the file itself uses it. Remove `export`; keep the code.
3. **Allow it** only when a real consumer sits outside the TypeScript import graph: an HTML
   entry, a URL string, a native build, a generated file. Add the narrowest allowance to
   `web/knip.ts` (an `entry`, `ignoreDependencies` or `ignoreFiles` item) with a comment that
   names the consumer.

The config treats knip's configuration hints as errors. An allowance that stops matching
anything fails the gate, so remove it in the same pull request.

Two allowances cover the folder-per-module convention:

- A module's barrel may re-export its types (`export type { XProps }`) whether or not a caller
  uses them yet.
- `ignoreFiles` lists module barrels that the structure test requires but no caller imports. The
  implementation behind each is still checked.

## deadcode (monthly Go report)

`make deadcode` runs `golang.org/x/tools/cmd/deadcode` (pinned in `scripts/deadcode.sh`, run
with `go run`) over `./cmd/...` and prints unreachable functions per package, then a total.
It is not a pull-request gate: it cannot see code reached only from build-tagged tests
(`ffmpeg`, `eval`, `integration`), so some of what it lists is intentional.

On the first of each month (or from **Actions → Go dead code report → Run workflow**), the
workflow compares the total with `docs/engineering/evidence/deadcode-baseline.txt`. If the total
grew, it opens the tracking issue "Go dead code grew past its baseline", or comments on it if
it's already open.

After deleting dead code, run `make deadcode-baseline` and commit the new baseline so the next
report measures from there.
