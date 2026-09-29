# expo-video 58.0.0-canary-20260812-27f94d4: one HTTP client per process

Tracking: [#1037](https://github.com/loomarr/loomarr/issues/1037) (G3 warm surf).

Upstream builds a new `OkHttpClient` for every video source, so every `replaceAsync` gets its own
connection pool and dispatcher. On Android TV each channel change therefore opened a new HTTPS
connection (DNS, TCP and TLS to the server's reverse proxy) before ExoPlayer could fetch the
channel's master playlist. A Perfetto trace of two warm surfs on an NVIDIA Shield measured the
master request at ~230 ms and ~830 ms, against 18–31 ms for each later request on the connection
once it was open: the new connection was most of the 600 ms warm budget.

The patch derives each source's client from one process-wide client with `newBuilder()`, which
shares the connection pool and dispatcher. Per-source settings (the cache interceptor, headers,
user agent) are unchanged. The playing channel polls its playlist every second, so the next
channel's first request finds a warm connection to the same server.

expo-video ships a precompiled Android AAR, which a source patch cannot reach, so the TV app opts
this package out of prebuilt modules (`expo.autolinking.android.buildFromSource` in
`web/apps/tv/package.json`). Remove both together when upstream reuses its client.
