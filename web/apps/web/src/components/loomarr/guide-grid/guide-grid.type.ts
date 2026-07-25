import type { GuideChannelTimeline } from "@loomarr/api";

interface GuideGridProps {
  // The generated wire shape, not a hand-written mirror: GET /v1/guide returns
  // GuideChannelTimeline[], each row carrying its airings with absolute epoch-ms times.
  channels: GuideChannelTimeline[];
  // The window being displayed, epoch ms. Echoed by the API (it clamps), so the grid draws
  // the window that was actually served rather than the one it asked for — otherwise a
  // clamped request would render blocks outside the axis.
  fromMs: number;
  toMs: number;
  // Pixels per minute. This is what makes the grid a GRID: every row is scaled by the same
  // factor against the same origin, so 9pm on row 1 is the same x as 9pm on row 5. Zoom is
  // just changing this number.
  pxPerMinute?: number;
  // "Now" as epoch ms, for the now-line. Injected rather than read from Date.now() inside
  // so stories and tests are deterministic, and so the caller can advance it on an interval
  // without this component owning a timer.
  nowMs?: number;
  onSelectAiring?: (channelId: string, startMs: number) => void;
  className?: string;
}

export type { GuideGridProps };
