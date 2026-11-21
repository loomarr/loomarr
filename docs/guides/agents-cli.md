# Use Loomarr from a CLI or an agent

**For:** people who want a script, a terminal, or an AI agent with a shell to use Loomarr.
**You'll get:** a token that acts as you, and [Restish](https://rest.sh) commands for every Loomarr API
operation.

A CLI or agent signs in the same way a TV app does: it **pairs as a device**. You approve it while
signed in, and from then on it acts as you, with your role. A member's agent can do what that
member can do, and nothing an admin can. If your role changes or your account is disabled, the
agent's next request sees it. You never hand it your password or the admin `API_TOKEN`.

**Approve as a member unless the agent needs admin routes.** An agent approved by an admin has
everything an admin session has, including revealing and regenerating secrets, and revoking it
later does not undo what it already read.

## Pair

Loomarr ships as a container image, so run the pairing from it. The token is the only thing
printed on stdout, which lets you capture it:

```sh
export LOOMARR_TOKEN=$(docker run --rm ghcr.io/loomarr/loomarr pair --server http://nas.local:8080)
```

Use the address you open Loomarr at, including any path prefix your reverse proxy adds. Inside
`docker run`, `localhost` is the container itself, so use the host's name or LAN address (or add
`--network host`). If your proxy redirects `http://` to `https://`, give the `https://` address. The
command prints a link and a code like `BCDF-GHJK`. Open the link, sign in as the person the agent
should act as, and approve. The command then prints the token and the Restish setup below.

`--server` can come from `LOOMARR_SERVER` instead. `--name "Laptop agent"` sets the label the device gets in the device list; the default is
`Agent CLI`. The code expires after 10 minutes; run `pair` again for a new one.

**Keep the token like a password.** Anyone holding it acts as you. Store it in your keyring or
password manager rather than a shell history or a shared file. On a plain-http address the token
crosses your network unencrypted, so keep that to a trusted LAN.

## Use it with Restish

[Install Restish](https://rest.sh) (2.x), then connect it to Loomarr once:

```sh
restish api connect loomarr http://nas.local:8080 --spec http://nas.local:8080/v1/openapi.json
restish api set loomarr 'profiles.default.auth.type: bearer' 'profiles.default.auth.params.token: env:LOOMARR_TOKEN'
```

`--spec` is required: Restish looks for the spec at `/openapi.json`, and Loomarr serves it under
`/v1`. The token is read from `LOOMARR_TOKEN` on each request, so it stays out of Restish's config.
`pair` prints both commands with your address filled in.

Each API operation becomes a command named after its operation ID:

```sh
restish loomarr --help            # every operation you can call
restish loomarr list-channels     # the channel list
```

An operation your role does not allow answers `403`, exactly as it would in the app.

## Revoke

Each person sees and removes **their own** paired devices:

- **From the agent itself:** `restish loomarr device-revoke-current` signs its token out for good.
- **From any of your sessions:** `restish loomarr device-list` shows your devices;
  `restish loomarr device-revoke <id>` removes one.
- **Admins** also see their own devices in Settings → **Sign-in, devices and sharing**.

An admin cannot list another person's devices. To cut off someone else's agent, disable their
account: that signs out all of their devices, and they stay signed out after the account is
enabled again.

After a revoke the token gets `401` on its next request. Pair again to get a new one.
