# expo-video 58.0.13: one HTTP client per process

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

58.0.13 only rebuilt the precompiled iOS binaries against React Native 0.88.1; its
`DataSourceUtils.kt` is byte-identical to 58.0.12's, so the patch is re-keyed unchanged.

## iOS: Swift Testing linkage (retired)

The SDK 58 canary's precompiled `ExpoVideo.xcframework` compiled its `CachePolicyTests` Swift Testing
suite into the framework, so `otool -L` listed `@rpath/Testing.framework/Testing` and the phone died
in dyld at launch; the phone therefore listed expo-video in `expo.autolinking.ios.buildFromSource`.
`llvm-otool -L` on all four 58.0.12 slices, and again on 58.0.13's rebuilt ones
(`prebuilds/output/{debug,release}`, device and Simulator), lists no Testing framework, while the
canary's release device slice still does. The explicit entry is gone. ExpoVideo still builds from source on iOS, because it depends on
ExpoModulesCore, which the phone builds from source, and Expo's precompiled-module resolver keeps
interdependent pods all prebuilt or all from source (`dependency_unavailable`).
