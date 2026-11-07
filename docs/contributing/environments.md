# Environments

**For:** contributors deciding where to try a change.
**You'll get:** what each environment proves, and what it can't.

| Environment | Runs | Proves | Doesn't prove |
| --- | --- | --- | --- |
| Worktree | A live-reload build with its own ports and database | The change works while you build it | The shipped image, or a real upgrade |
| Pull request | Fast affected checks | The change compiles and its unit tests pass | Browser suites, race tests, Postgres or the image |
| Merge queue | Every affected gate on the current base | The change is safe to merge | That the image runs as a deployed app |
| Demo site | A real image beside the invented demo library | The image starts, seeds, and plays channels | Real data, GPU encoding or load |
| Staging | The demo site on a cloud server, for a few hours | The same as the demo site, away from your machine, from a commit or a release | Real data, GPU encoding or load |
| Production | A signed release | Everything, on real data | Nothing: a mistake here reaches a household |

## Run the demo site

```bash
make demo-site VERSION=0.2.0-beta.9-rc.11
```

That runs the published release with six invented channels. Everything on it is generated, so it
needs no media server, LLM or TMDB key. The first start takes about four minutes, then it prints the
address. Sign in as `demo` with the password `loomarr-demo`.

To run an image you built from this checkout, name it instead:

```bash
make demo-site LOOMARR_IMAGE=<image>
```

The image and the demo tool must come from the same commit. The tool migrates the database, and a
server won't open a database newer than itself. `VERSION` handles this by building the tool from
that release's tag.

`make demo-site-down` stops it and deletes its data. Run it before switching to an older version,
because the data stays between runs and an older server won't open it.

The sign-in is public, so the site only listens on this machine. Set `DEMO_SITE_BIND` only on a
private network. Without a GPU it plays two channels at once, and a third is refused until one
stops. That's Loomarr's capacity limit working, not a fault.

## Run staging

Staging is the demo site on a cloud server. It runs for a few hours, then it's destroyed, because
the provider keeps billing a stopped server. It costs about $0.06 an hour.

```bash
gh workflow run staging.yml -f version=0.2.0-beta.9-rc.11 -f hours=4
```

Leave `version` out to build the commit the run is on: add `--ref <branch>`, and allow about ten
more minutes for the image. When it has booted, open the address the run prints from a device on
the private network. Destroy it early with `-f action=down`.

Only one server runs at a time. An hourly sweep destroys anything past its time, even if the run
that made it failed.

### Keep the demo site running

Use `-f hours=keep` for the long-running demo site. The sweep leaves it alone, and it runs until
`-f action=down`, at about $40 a month. To move it to a new version, take it down and start it
again. While it's up, that one server is also staging, because only one runs at a time.

The workflow only ever acts on a server with both its tag and its name prefix, because the cloud
key can reach everything in the account. Keep both checks in `scripts/staging.sh`.

### One-time setup

| Setting | What it needs |
| --- | --- |
| `VULTR_API_KEY` | A repository secret: a key for the cloud account |
| `STAGING_TS_AUTHKEY` | A repository secret: a reusable, ephemeral, pre-approved tailnet key |
| `ghcr.io/loomarr/loomarr-staging` | Set this package public after its first build, so the server can pull it |
