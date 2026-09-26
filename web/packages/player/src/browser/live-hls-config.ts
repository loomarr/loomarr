import type { HlsConfig } from "hls.js";

/**
 * hls.js policy for a live Loomarr channel, tuned to the channel packager (#1512 phase 2): 1 s fMP4
 * segments, listed only up to wall-clock now + 6 s, advertised as EXT-X-SERVER-CONTROL:HOLD-BACK=6.
 */
const liveHlsConfig = {
  // A source-scoped controller stays empty until its transferred MediaSource is attached. The
  // handoff then loads the source and performs one explicit media start.
  autoStartLoad: false,
  capLevelToPlayerSize: true,
  // Baseline HLS is MPEG-TS. Keep its transmux off the UI thread; hls.js shares and reference-
  // counts this worker across the bounded source-scoped controller pair.
  enableWorker: true,
  // Live channel: keep chasing the live edge, and be patient while it warms up. A cold channel may
  // briefly list no media while its encoder starts, so hls.js must RETRY, not give up.
  liveDurationInfinity: true,
  manifestLoadingMaxRetry: 8,
  manifestLoadingRetryDelay: 1000,
  levelLoadingMaxRetry: 8,
  fragLoadingMaxRetry: 8,
  // The packager serves no partial segments; LL-HLS (hls.js's default) would only add blocking
  // playlist reloads and part hold-back to a stream that has neither.
  lowLatencyMode: false,
  // ⚠ 6 s = the packager's HOLD-BACK and listing gate. The newest listed segment is now + 6 s, so
  // this plays wall-clock now: the picture matches the guide and the tune-in still (decoded at the
  // airing's "now"), and the 6 s between the playhead and the edge are already encoded (run-ahead
  // leads by up to 12 s), so the forward buffer fills at network speed with no wait on the encoder.
  // A shorter value would only give that cushion away; a cold tune lists less than 6 s and hls.js
  // starts from the first listed segment either way.
  liveSyncDuration: 6,
  // The shared DVR window, not hls.js's latency correction, decides when an intentional pause
  // expires. Keep this above the complete fifteen-minute server horizon. Seconds, not a segment
  // count: hls.js refuses to mix the two forms.
  liveMaxLatencyDuration: 10_000,
  // Request the first fragment while the manifest is parsed rather than after: one round trip
  // less between the manifest and the first frame.
  startFragPrefetch: true,
  // Build a forward cushion after fast start and retain the complete shared DVR horizon.
  maxBufferLength: 60,
  backBufferLength: 900,
} as const satisfies Partial<HlsConfig>;

export { liveHlsConfig };
