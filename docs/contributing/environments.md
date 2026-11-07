# Environments

**For:** contributors deciding where to try a change.
**You'll get:** what each environment proves, and what it can't.

| Environment | Runs | Proves | Doesn't prove |
| --- | --- | --- | --- |
| Worktree | A live-reload build with its own ports and database | The change works while you build it | The shipped image, or a real upgrade |
| Pull request | Fast affected checks | The change compiles and its unit tests pass | Browser suites, race tests, Postgres or the image |
| Merge queue | Every affected gate on the current base | The change is safe to merge | That the image runs as a deployed app |
| Demo site | A real image beside the invented demo library | The image starts, seeds, and plays channels | Real data, GPU encoding or load |
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
