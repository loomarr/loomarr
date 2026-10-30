// PROTOTYPE — throwaway (#2088 step 0). The contract every /prototype/watch variant renders against:
// one named state at a time (forced via ?state=), on one device frame (?device=).

const WATCH_STATES = [
  "playing", // controls shown over the picture
  "idle", // controls auto-hidden: picture only
  "station-id", // a settled tune: the 5 s station-ID card
  "tuning", // the tuning wash between channels
  "behind", // paused / seeked into the 15-min window: N behind · Go live · Start over
  "commercials", // a filler break airing: "Commercials · <clip title>"
  "info", // the I / ⓘ info sheet with up-next
  "settings", // the ⚙ menu: quality · audio · subtitles · playback info
  "playback-info", // the Playback info overlay (admin view)
  "drawer", // the G channels drawer
  "help", // the ? shortcut sheet
  "error", // the stream stopped: message + Retry
  "off-air", // channel paused: nothing to play
] as const;
type WatchState = (typeof WATCH_STATES)[number];

const DEVICES = ["desktop", "portrait", "landscape"] as const;
type Device = (typeof DEVICES)[number];

interface VariantProps {
  state: WatchState;
  device: Device;
}

export type { Device, VariantProps, WatchState };
export { DEVICES, WATCH_STATES };
