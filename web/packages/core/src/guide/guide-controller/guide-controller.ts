import { getChannelGuideUrl } from "@loomarr/api/endpoints/channels";
import type { GuideOutputBody } from "@loomarr/api/models/guideOutputBody";

import { defaultGuideWindow, guideSelectionForChannel, layoutGuide, moveGuideSelection } from "../guide";
import type { GuideLayout, GuideWindow } from "../guide.type";
import type {
  GuideController,
  GuideControllerOptions,
  GuideControllerSnapshot,
  GuideSourcePort,
} from "./guide-controller.type";

const createGuideSourcePort = (request: typeof globalThis.fetch): GuideSourcePort => ({
  load: async (window, signal) => {
    const response = await request(getChannelGuideUrl(window), { method: "GET", signal });
    if (!response.ok) throw new Error(`Couldn't load the Guide (${response.status}).`);
    return (await response.json()) as GuideOutputBody;
  },
});

const createGuideController = ({
  now = Date.now,
  resolveWindow = defaultGuideWindow,
  source,
}: GuideControllerOptions): GuideController => {
  let disposed = false;
  let request: AbortController | undefined;
  let snapshot: GuideControllerSnapshot = { status: "loading" };
  // The whole served guide; the snapshot's layout is this, narrowed to `only` when a filter is set.
  let full: GuideLayout | undefined;
  let only: ReadonlySet<string> | undefined;
  // The window a read asked for and did not get. A failed page leaves the window the viewer paged
  // away from on screen, but the Notice's Try again must read the one they asked for; it is kept
  // until a read of it lands, or the viewer asks for another window.
  let failedWindow: GuideWindow | undefined;
  const listeners = new Set<() => void>();

  const publish = (next: GuideControllerSnapshot) => {
    snapshot = next;
    for (const listener of listeners) listener();
  };

  const narrow = (layout: GuideLayout): GuideLayout =>
    only
      ? { ...layout, channels: layout.channels.filter((channel) => only?.has(channel.source.channelId)) }
      : layout;

  // Keep the requested channel when it's in view, else the first row, at the same time column.
  // `carry` is the refresh state to keep: a filter change re-settles the same stale or re-reading
  // Guide, so its error and its in-flight flag still hold; a fresh load starts with neither.
  const settle = (
    layout: GuideLayout,
    channelId: string | undefined,
    anchorMs: number,
    carry: Pick<GuideControllerSnapshot, "error" | "refreshing"> = {},
  ) => {
    const requested = channelId ? guideSelectionForChannel(layout, channelId, anchorMs) : undefined;
    const first = layout.channels[0]?.source.channelId;
    const selection = requested ?? (first ? guideSelectionForChannel(layout, first, anchorMs) : undefined);
    publish({ ...carry, layout, selection, status: layout.channels.length ? "ready" : "empty" });
  };

  const loadWindow = async (
    window: GuideWindow,
    preferredChannelId: string | undefined,
    anchorMs: number,
    // Whether a failure keeps `window` to be read again: a window the viewer chose does, the live
    // one (which moves on with the clock) does not.
    remember: boolean,
  ) => {
    if (disposed) return;
    request?.abort();
    const nextRequest = new AbortController();
    request = nextRequest;
    // A shown layout stays on screen while it is re-read; only the very first load reports
    // `loading`. A failed read's error holds until the re-read answers, for a shown layout and for a
    // first load alike: clearing it at once would unmount the notice or error panel that offered
    // "Try again" while the viewer's focus is on it.
    const shown = snapshot.layout !== undefined;
    const retryingFirstLoad = !shown && snapshot.status === "error";
    publish(
      shown || retryingFirstLoad
        ? { ...snapshot, refreshing: true }
        : { ...snapshot, error: undefined, status: "loading" },
    );

    try {
      const sourceGuide = await source.load(window, nextRequest.signal);
      if (disposed || nextRequest.signal.aborted || request !== nextRequest) return;

      full = layoutGuide(sourceGuide, now());
      failedWindow = undefined;
      settle(narrow(full), preferredChannelId, anchorMs);
    } catch (error) {
      if (disposed || nextRequest.signal.aborted || request !== nextRequest) return;
      const message = error instanceof Error ? error.message : "Couldn't load the Guide.";
      failedWindow = remember ? window : undefined;
      publish(
        shown
          ? { ...snapshot, error: message, refreshing: undefined }
          : { ...snapshot, error: message, refreshing: undefined, status: "error" },
      );
    }
  };

  return {
    dispose: () => {
      if (disposed) return;
      disposed = true;
      request?.abort();
      listeners.clear();
    },
    getSnapshot: () => snapshot,
    move: (direction) => {
      if (disposed || snapshot.status !== "ready" || !snapshot.layout || !snapshot.selection) {
        return undefined;
      }
      const result = moveGuideSelection(snapshot.layout, snapshot.selection, direction);
      if (!result.boundary) publish({ ...snapshot, selection: result.selection });
      return result;
    },
    page: async (direction) => {
      if (disposed || !full) return;
      const span = full.toMs - full.fromMs;
      const liveFrom = resolveWindow(now()).from;
      const candidateFrom = direction === "earlier" ? full.fromMs - span : full.fromMs + span;
      const from = direction === "earlier" ? Math.max(liveFrom, candidateFrom) : candidateFrom;
      if (from === full.fromMs) return;
      await loadWindow(
        { from, to: from + span },
        snapshot.selection?.channelId,
        snapshot.selection?.anchorMs ?? now(),
        true,
      );
    },
    // Re-reads the page the viewer is on (a channel event, a reconnect and a tune all refresh), so
    // only a viewer at the live window follows now. Paging never goes before live, so a page that
    // starts after the live window is one the viewer chose; once now reaches it, live takes over. A
    // window that failed to load is the one they asked for, so it is read again before any other.
    refresh: async (preferredChannelId) => {
      const at = now();
      const live = resolveWindow(at);
      const retrying = failedWindow;
      await loadWindow(
        retrying ?? (full && full.fromMs > live.from ? { from: full.fromMs, to: full.toMs } : live),
        preferredChannelId ?? snapshot.selection?.channelId,
        snapshot.selection?.anchorMs ?? at,
        retrying !== undefined,
      );
    },
    restrict: (channelIds) => {
      if (disposed) return;
      only = channelIds ? new Set(channelIds) : undefined;
      if (!full || (snapshot.status !== "ready" && snapshot.status !== "empty")) return;
      settle(narrow(full), snapshot.selection?.channelId, snapshot.selection?.anchorMs ?? now(), {
        error: snapshot.error,
        refreshing: snapshot.refreshing,
      });
    },
    select: (selection) => {
      if (disposed || snapshot.status !== "ready") return;
      publish({ ...snapshot, selection });
    },
    subscribe: (listener) => {
      if (disposed) return () => undefined;
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
};

export { createGuideController, createGuideSourcePort };
