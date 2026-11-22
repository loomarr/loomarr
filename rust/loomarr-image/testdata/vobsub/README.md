# VOBSUB fixture

`track.idx` and `track.sub` are a real `dvd_subtitle` pair: ffmpeg's `dvdsub` encoder in Matroska,
then mkvextract's VobSub writer, since ffmpeg has no VobSub muxer. ffmpeg encodes only bitmap
subtitles to `dvdsub`, so the source is `source.sup`, a 720x480 PGS track of two display sets:
a 40x10 object at (100, 400) from 1 s to 2.5 s and a 64x8 object at (200, 300) from 3 s to 4 s,
each with white rows over black ones.

```sh
ffmpeg -copyts -fix_sub_duration -i source.sup -c:s dvdsub track.mkv
mkvextract tracks track.mkv 0:track.idx
```

`-fix_sub_duration` gives each subpicture its real stop; without it ffmpeg codes the open PGS
duration as the longest stop delay.
