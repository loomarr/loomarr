# Installing Loomarr

[Docker](docker.md) is the step-by-step. This page covers what Loomarr needs and the one
choice you'll be asked to make.

## What you need

Only **Emby or Jellyfin** is required for Loomarr to start and serve an existing channel. The
product's defining describe-a-channel flow also requires **TMDB** and an **LLM** (local Ollama or
an OpenAI-compatible provider). Requesters and filler add capabilities; Loomarr degrades without
them rather than refusing to start.

```mermaid
graph LR
  subgraph required[" "]
    MS["<b>Media server</b><br/>Emby / Jellyfin<br/><i>your library</i>"]
  end
  L["<b>Loomarr</b>"]
  subgraph optional[" "]
    direction TB
    AI["<b>LLM</b> + <b>TMDB</b><br/><i>to suggest</i>"]
    RQ["<b>Seerr</b> or <b>Sonarr/Radarr</b><br/><i>to download</i>"]
    FI["<b>Filler folder</b><br/><i>for commercials</i>"]
  end
  V["<b>Your TVs & browsers</b>"]

  MS -->|reads library| L
  AI -.-> L
  RQ -.-> L
  FI -.-> L
  L ==>|Live TV tuner + guide| MS
  MS ==> V

  classDef req fill:#1f6f4a,stroke:#134a31,color:#fff
  classDef opt fill:#2b3b52,stroke:#1b2736,color:#dbe4ef,stroke-dasharray:4 3
  classDef core fill:#8a5a1a,stroke:#5c3b10,color:#fff
  class MS req
  class AI,RQ,FI opt
  class L core
  style required fill:none,stroke:none
  style optional fill:none,stroke:none
```

Without an LLM and a TMDB key, Loomarr can't suggest lineups. Without a requester, it schedules
only what you already own. Without a filler folder, channels play back to back.

## Who streams your channels

The wizard asks this early because it changes what else you set up.

```mermaid
graph TD
  Q{"Who streams<br/>your channels?"}
  Q -->|"<b>Loomarr</b> — default"| I["Loomarr encodes and serves<br/>its own tuner + guide"]
  Q -->|Tunarr| T["Loomarr sends the schedule<br/>to Tunarr, which streams"]

  I --> I1["Nothing extra to install"]
  I --> I2["Ad breaks can go mid-programme"]
  I --> I3["Channels stop if Loomarr stops"]

  T --> T1["You install and run Tunarr"]
  T --> T2["Good if hardware can't transcode"]
  T --> T3["Channels keep playing without Loomarr"]

  classDef def fill:#1f6f4a,stroke:#134a31,color:#fff
  classDef alt fill:#2b3b52,stroke:#1b2736,color:#dbe4ef
  classDef note fill:none,stroke:none,color:#8fa3bf
  class I def
  class T alt
  class I1,I2,I3,T1,T2,T3 note
```

Pick Loomarr unless you have a reason not to. You can change it later, and override it per
channel.

## What runs where

Loomarr is one compiled application container behind a Traefik edge container. Traefik publishes
**port 8080** by default; Loomarr serves UI, API, and streams on private container port 8080 and
stores state under **`/data`**.

| It needs | For |
| --- | --- |
| Host port 8080 | Traefik routes UI, API, and video streams to Loomarr |
| A `/data` volume | Database, artwork, filler clips |
| `SERVER_PUBLIC_URL` | The address your media server reaches Loomarr on |
| A GPU (optional) | Hardware encoding — software works, just fewer channels at once |

Set `SERVER_PUBLIC_URL` to something your media server can resolve, like a LAN IP. Release Compose
refuses to start without it; a wrong but reachable-looking value still shows up when a channel
fails to play. The beta supports one Loomarr replica behind Traefik while multi-replica safety is
tested explicitly.

The published artifact is a Linux container for amd64 and arm64. Linux runs it directly. macOS
runs the same image through Docker Desktop; Apple Silicon selects arm64 automatically. Docker
Desktop does not pass through the Mac GPU to Linux containers, so internal playout uses software
encoding there.

## Next

- [Docker install](docker.md)
- [Hardware acceleration](hardware.md)
- [Upgrading](upgrading.md)
- [All settings](../configuration.md)
