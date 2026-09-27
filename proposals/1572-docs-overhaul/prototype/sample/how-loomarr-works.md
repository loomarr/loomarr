# How Loomarr works

**For:** anyone who wants the big picture before changing settings or code.
**You'll get:** Loomarr's parts, and what each one talks to.

![People send a sentence to Loomarr. Inside Loomarr, the Suggester, Channel builder, Scheduler and Playout run in sequence, with Filler feeding the Scheduler. The Suggester uses TMDB and an LLM, the Channel builder asks Seerr or Sonarr and Radarr for missing titles, and Playout serves channels to Emby or Jellyfin as Live TV, which TVs and browsers watch.](../diagrams/generated/architecture.svg)

Loomarr is one container with one database, SQLite or Postgres. Every part keeps its state
there.

- **Suggester.** Turns your sentence into a proposal. It looks titles up in your library and
  TMDB, and uses the LLM only to choose among real titles.
- **Channel builder.** Runs after an admin approves. It creates the channel and asks your
  requester for anything missing.
- **Scheduler.** Decides what airs when, and slots in breaks from **Filler**.
- **Playout.** Encodes a channel only while someone watches it, and hands it to your media
  server as Live TV. [How on-demand playout works](playout.md).
