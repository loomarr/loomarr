import type {
  GuideLayout,
  GuideNavigationDirection,
  GuideNavigationResult,
  GuideSelection,
  GuideWindow,
} from "../guide.type";

type GuideControllerStatus = "empty" | "error" | "loading" | "ready";

interface GuideSourcePort {
  load: (window: GuideWindow, signal: AbortSignal) => Promise<GuideLayout["source"]>;
}

interface GuideControllerSnapshot {
  error?: string;
  layout?: GuideLayout;
  selection?: GuideSelection;
  status: GuideControllerStatus;
}

interface GuideController {
  dispose: () => void;
  getSnapshot: () => GuideControllerSnapshot;
  move: (direction: GuideNavigationDirection) => GuideNavigationResult | undefined;
  /**
   * Pages the served window one window-length earlier or later (#1659 decision N4): a ten-foot
   * platform's only way to see schedule outside the default window. Paging earlier clamps to the
   * live window — it never serves a window that starts before now.
   */
  page: (direction: "earlier" | "later") => Promise<void>;
  refresh: (preferredChannelId?: string) => Promise<void>;
  /**
   * Show only these channels, in guide order (the Favorites and Recent filters); undefined shows all.
   * Moves and selection follow the filtered rows, so the D-pad never lands on a hidden channel.
   */
  restrict: (channelIds: readonly string[] | undefined) => void;
  select: (selection: GuideSelection) => void;
  subscribe: (listener: () => void) => () => void;
}

interface GuideControllerOptions {
  now?: () => number;
  resolveWindow?: (at: number) => GuideWindow;
  source: GuideSourcePort;
}

export type {
  GuideController,
  GuideControllerOptions,
  GuideControllerSnapshot,
  GuideControllerStatus,
  GuideSourcePort,
};
