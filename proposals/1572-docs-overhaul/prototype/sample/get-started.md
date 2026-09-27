# Get started

**For:** anyone installing Loomarr for the first time.
**You'll get:** Loomarr running, and one channel of your own playing in your media server's
Live TV guide. It takes about 10 minutes.

## Before you start

You need three things running or ready:

- **Emby or Jellyfin**, with an admin API key.
- **A TMDB API key.** It's free: [get one from TMDB](https://www.themoviedb.org/settings/api).
- **An LLM that supports tool calling.** A local Ollama works, and so does any
  OpenAI-compatible provider.

You also need Docker, on Linux or on Docker Desktop for macOS.

> [!NOTE]
> Downloading missing titles (Seerr, or Sonarr and Radarr), commercials between shows and
> hardware encoding are all optional. You can add them later from **Settings**.

## 1. Start Loomarr

Download the release you want to run and start it. This example uses `v0.2.0-beta.7`; check
[Releases](https://github.com/loomarr/loomarr/releases) for a newer one.

```bash
VERSION=0.2.0-beta.7
git clone --branch "v${VERSION}" --depth 1 https://github.com/loomarr/loomarr
cd loomarr
cp .env.example .env
```

Open `.env` and set one value: the address your media server will use to reach Loomarr.

```bash
SERVER_PUBLIC_URL=http://192.168.1.10:8080
```

Use your computer's LAN address, not `localhost`. Inside Docker, `localhost` means the
container itself, so your media server couldn't reach it.

Then start it:

```bash
LOOMARR_VERSION="$VERSION" docker compose -f docker/compose.yaml --profile sqlite up -d
```

When this prints `ready`, Loomarr is up:

```bash
curl -fsS http://localhost:8080/v1/readyz && echo ready
```

## 2. Run the setup wizard

Open the address you set in `SERVER_PUBLIC_URL`. The wizard walks you through six steps:

1. **Admin:** create your admin account.
2. **Playout:** choose who streams your channels. Keep **Loomarr**, the default.
3. **Location:** pick the country where you watch. Loomarr uses it for channels and
   commercials.
4. **Connections:** enter your media server, TMDB and LLM. Each one is tested as you go.
5. **Users** (optional): choose who else can sign in.
6. **First channel:** go on to the next step of this guide.

> [!TIP]
> If a connection turns red, the message links to the fix. You can leave the wizard and
> come back later; it remembers where you were.

## 3. Describe your first channel

Type what you want to watch, as you'd say it to a friend:

> 90s Saturday morning cartoons for the kids

Loomarr searches your library and TMDB, then shows a **proposal**: the lineup it would
play, and anything you don't have yet. Every title in it is real. The model can't invent one.

Choose **Approve**. That creates the channel.

## 4. Watch it

Open your media server's **Live TV** guide. Your channel appears within a minute. Pick it, and
it starts playing.

Loomarr encodes a channel only while someone is watching it, so the first picture takes a few
seconds.

## What's next

- [Set up the TV app](guides/tv-app.md)
- [Add hardware encoding](guides/hardware-encoding.md) to run more channels at once
- [Add commercials between shows](guides/filler.md)
- [How on-demand playout works](explanation/playout.md)
