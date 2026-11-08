# Environments

**For:** contributors deciding where to try a change.
**You'll get:** what each environment proves, and what it can't.

| Environment | Runs | Proves | Doesn't prove |
| --- | --- | --- | --- |
| Worktree | A live-reload build with its own ports and database | The change works while you build it | The shipped image, or a real upgrade |
| Pull request | Fast affected checks | The change compiles and its unit tests pass | Browser suites, race tests, Postgres or the image |
| Merge queue | Every affected gate on the current base | The change is safe to merge | That the image runs as a deployed app |
| Demo site | A real image beside the demo library | The image starts, seeds, and plays channels | Real data, GPU encoding or load |
| Staging | The demo site on a cloud server, for a few hours | The same as the demo site, away from your machine, from a commit or a release | Real data, GPU encoding or load |
| Migration rehearsal | A candidate image on a copy of a real backup, with no network | The migrations run on real data and the server starts | That the app behaves well afterwards |
| Production | A signed release | Everything, on real data | Nothing: a mistake here reaches a household |

## Run the demo site

```bash
make demo-site VERSION=0.2.0-beta.9-rc.11
```

That runs the published release with six invented channels. Everything on it is generated, so it
needs no media server, LLM or TMDB key. The first start takes about four minutes, then it prints the
address. Sign in as `demo` with the password `loomarr-demo`.

It runs in demo mode (`LOOMARR_DEMO_USERNAME` and `LOOMARR_DEMO_PASSWORD`). The sign-in screen offers
that account in one press, and a banner at the top of every page says it's the demo and suggests
three things to try. A release from before demo mode ignores both settings.

To run an image you built from this checkout, name it instead:

```bash
make demo-site LOOMARR_IMAGE=<image>
```

The image and the demo tool must come from the same commit. The tool migrates the database, and a
server won't open a database newer than itself. `VERSION` handles this by building the tool from
that release's tag.

It uses SQLite. Add `DEMO_SITE_DATABASE=postgres` to run it on Postgres instead:

```bash
DEMO_SITE_DATABASE=postgres make demo-site VERSION=0.2.0-beta.9-rc.11
```

`make demo-site-down` stops it and deletes its data. Run it before switching to an older version or
a different database, because the data stays between runs and an older server won't open it.

Add `DEMO_SITE_CONTENT=open` to fill it with real films, cartoons and vintage commercials instead of
the invented library: the [open content pack](demo-content.md), every item openly licensed. The
first start downloads about 1.3 GB and checks every file against its pinned hash. The public site
uses it by default. Run `make demo-site-down` before switching between the two.

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
the private network. Destroy it early with `-f action=down`. Add `-f database=postgres` to run it on
Postgres.

Only one server runs at a time. An hourly sweep destroys anything past its time, even if the run
that made it failed.

### Keep the demo site running

Use `-f hours=keep` for the long-running demo site. The sweep leaves it alone, and it runs until
`-f action=down`, at about $40 a month. To move it to a new version, take it down and start it
again. While it's up, that one server is also staging, because only one runs at a time.

The workflow only ever acts on a server with both its tag and its name prefix, because the cloud
key can reach everything in the account. Keep both checks in `scripts/staging.sh`.

### Put it on the internet

Add `-f public_url=https://demo.loomarr.media` to reach it through a Cloudflare Tunnel. No port opens
on the server, and the address keeps working when the server is rebuilt.

The `demo` admin password is in this repository, so it can't be left working on a public site. Before
the tunnel starts, `scripts/demo-site.sh` creates a `guest` member account with the password
`loomarr-guest`, replaces the admin password with `STAGING_DEMO_ADMIN_PASSWORD`, and checks the old
one is refused. If any step fails, the tunnel doesn't start. Sign in as `demo` with that password to
manage the site. The sign-in screen offers visitors `guest`, never the admin.

Visitors share `guest`, so a Cloudflare rule blocks `/v1/auth/password` to stop anyone changing its
password. Keep that rule. The site plays two channels at once, so a third visitor is refused until
one stops.

### One-time setup

| Setting | What it needs |
| --- | --- |
| `VULTR_API_KEY` | A repository secret: a key for the cloud account, allowing all IPs because GitHub's runners change address |
| `STAGING_TS_AUTHKEY` | A repository secret: a reusable, ephemeral, pre-approved tailnet key with a tag. Sign it first if the tailnet uses Tailnet Lock |
| `STAGING_CF_TUNNEL_TOKEN` | A repository secret: the token for a Cloudflare Tunnel that routes the public address to `http://loomarr:8080` |
| `STAGING_DEMO_ADMIN_PASSWORD` | A repository secret: the public site's admin password, 16 or more letters, digits or `._~-` |
| `ghcr.io/loomarr/loomarr-staging` | Set this package public after its first build, so the server can pull it |

The tag stops the server's login expiring, which would take a long-running site offline. Treat the
server as untrusted: let your devices reach its web port, and give the tag no access to anything else
on the tailnet. The key lasts 90 days at most. A running server is unaffected when it expires, but
the next `up` fails until you replace it.
