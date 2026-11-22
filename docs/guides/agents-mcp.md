# Connect an AI agent with MCP

**For:** people who want Claude Code, or another AI app that speaks MCP, to use Loomarr.
**You'll get:** Loomarr's tools in your agent: what's on, the guide, stars, channel requests, and for an admin, health and approvals.

MCP is the Model Context Protocol, the standard way an AI app is given tools.

An MCP agent signs in the same way the [CLI](agents-cli.md) does: it **pairs as a device** and
acts as the person who approved it. Each tool calls the same API the apps use, as that person, so
an agent can do only what they can. A member's agent is not even shown the admin tools.

**Approve as a member unless the agent needs the admin tools.** An admin's agent can approve
channel requests and run jobs.

## Get a token

Pair once, from the release image, and keep the token it prints:

```sh
export LOOMARR_TOKEN=$(docker run --rm ghcr.io/loomarr/loomarr pair --server http://nas.local:8080 --name "Claude Code")
```

[Pair](agents-cli.md#pair) covers the address to use, the approval link and the 10-minute code.

## Add it to Claude Code

```sh
claude mcp add --transport http loomarr http://nas.local:8080/v1/mcp --header "Authorization: Bearer $LOOMARR_TOKEN"
```

Then ask, for example, "what's on channel 4 tonight?" or "star the cartoons channel". Any other
MCP client works the same way if it supports the Streamable HTTP transport with a custom
`Authorization` header: the endpoint is `<your Loomarr address>/v1/mcp`.

## Connect the Claude apps, or any client that signs in

The Claude desktop, web and mobile apps add Loomarr as a **custom connector** and sign in with
OAuth instead of a pasted token. So does any MCP client that follows the MCP authorization spec.

1. Loomarr must be reachable from the internet over **HTTPS** (for example through your reverse
   proxy or a tunnel). The apps' connectors call it from Anthropic's servers, not from your
   network. A client on your LAN only needs an address it can reach.
2. Set Loomarr's public address: **Recipient-facing Loomarr address** (`ACCESS_PUBLIC_URL`), or the
   server's public URL if that is the address people use. Sign-in is off until one is set.
3. In the app, add a custom connector with the URL `https://<your Loomarr address>/v1/mcp`.
4. The app opens Loomarr's **Add a device** page with the code filled in. Sign in as the person
   the agent should act as. The page names the app and the address it sends the sign-in to, for
   example `claude.ai`. Add it only if that is the app you just connected: it can do everything
   that person can.

The connection is a paired device like any other, named for the app and where it sends its
sign-in, for example **Claude (claude.ai)**. It does not expire; revoke the device to end it.

## Tools

| Tool | Who | What it does |
| --- | --- | --- |
| `list_channels` | everyone | the channels by number and name |
| `whats_on` | everyone | now and next on every channel, or the next few programmes on one |
| `tonight` | everyone | premieres and marathons in the next hours |
| `guide` | everyone | the programme guide from now on, for every channel or one |
| `watch_link` | everyone | a short-lived stream link to open in a player such as VLC |
| `my_channels` | everyone | your starred and recent channels |
| `star_channel`, `unstar_channel` | everyone | star or unstar a channel |
| `channel_ideas` | everyone | ideas for new channels from your library |
| `request_channel_idea` | members | ask for one of those ideas; an admin approves it |
| `propose_channel` | everyone | ask for a channel described in words; an admin approves it |
| `who_is_watching` | everyone | who is watching what; a member sees only their own devices |
| `health` | admins | the health checks that are not passing |
| `why_not_playing` | admins | why a channel is not playing, or how every live stream is doing |
| `needs_attention` | admins | requests waiting for approval, failing jobs, library decisions |
| `recent_events` | admins | recent warnings and errors, or events of one level |
| `list_jobs`, `job_history`, `run_job` | admins | the scheduled jobs, their runs, and running one now |
| `approve_proposal`, `deny_proposal` | admins | answer a channel request |

Channels are named the way you would say them: a number (`4`, `channel 4`) or a name or part of
one. If a name matches more than one channel, the tool lists them and asks which you meant. Times
are in your household's time zone.

## Revoke

The agent is a paired device like any other: [revoke it](agents-cli.md#revoke) and its next
request gets `401`. Every request is checked afresh, so a revoked token, a disabled account or a
role change applies at once, with no session left open.

On a plain-http address the token crosses your network unencrypted, so keep that to a trusted LAN.
