# How on-demand playout works

**For:** operators sizing hardware, and anyone wondering why a channel takes a few seconds to
start.
**You'll get:** what happens between choosing a channel and seeing a picture.

![When someone tunes in, the channel packager starts. It asks the schedule what airs now, starts one ffmpeg process for that item reading your media file directly, and adds the encoded fragments to its timeline. Slate fills the timeline if an item is late. The one timeline is served as HLS to browsers and the TV app, and as MPEG-TS to the Emby or Jellyfin tuner.](../diagrams/generated/on-demand-playout.svg)

**No viewer, no encoder.** Nothing is encoded ahead of time. The first tune starts the
channel's packager, and it stops 30 seconds after the last viewer leaves.

- **One packager per channel and format.** Everyone watching a channel shares one encode,
  whether they're in a browser, the TV app or your media server.
- **One ffmpeg per item.** Each programme, break and card gets its own encoder. Every item is
  encoded to the same format, so the switch between them is seamless.
- **Slate, never black.** If the next item isn't ready in time, a slate plays, and the
  programme rejoins in progress.
