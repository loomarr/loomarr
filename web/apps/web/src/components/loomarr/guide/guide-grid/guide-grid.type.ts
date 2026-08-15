import type { GuideAiring, GuideChannelTimeline } from "@loomarr/api";
import type { ReactNode } from "react";

interface GuideGridProps {
  // The generated wire shape, not a hand-written mirror: GET /v1/guide returns
  // GuideChannelTimeline[], each row carrying its airings with absolute epoch-ms times.
  channels: GuideChannelTimeline[];
  // The window being displayed, epoch ms. Echoed by the API (it clamps), so the grid draws
  // the window that was actually served rather than the one it asked for.
  fromMs: number;
  toMs: number;
  // Zoom scales the CHROME — rail width, row height, type — not the time scale. The window
  // always fits the viewport, which is the TV-guide convention: you change how much detail
  // each row shows, not how much time is on screen. (Time span is chosen separately, by
  // asking the API for a different window.)
  zoom?: number;
  // "Now" as epoch ms, for the now-line and the airing highlight. Injected rather than read
  // from Date.now() inside, so stories and tests are deterministic and the caller owns the
  // ticking.
  nowMs?: number;
  // The actual inspected block is the positioning anchor. Passing the element lets a floating
  // positioner follow horizontal/vertical scrolling and resolve viewport collisions from real
  // geometry; a percentage + row-index estimate cannot know the card or browser height.
  anchor?: HTMLElement;
  // Called when a block is hovered/focused — the caller renders the detail card. Null clears.
  onInspect?: (airing: GuideAiring | null, channelId?: string, anchor?: HTMLElement) => void;
  onSelectChannel?: (channelId: string) => void;
  // Renders the per-row actions menu (edit / pause / delete). A RENDER PROP rather than a
  // built-in menu: those actions are admin-only mutations that need the store, mutations and
  // confirm dialogs, and pulling them in here would make a presentational grid depend on all
  // of it. The page passes the real menu; stories and tests pass nothing.
  renderRowMenu?: (channel: GuideChannelTimeline) => ReactNode;
  className?: string;
}

export type { GuideGridProps };
