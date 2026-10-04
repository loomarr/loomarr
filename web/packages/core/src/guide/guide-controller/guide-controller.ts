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
  const settle = (layout: GuideLayout, channelId: string | undefined, anchorMs: number) => {
    const requested = channelId ? guideSelectionForChannel(layout, channelId, anchorMs) : undefined;
    const first = layout.channels[0]?.source.channelId;
    const selection = requested ?? (first ? guideSelectionForChannel(layout, first, anchorMs) : undefined);
    publish({ layout, selection, status: layout.channels.length ? "ready" : "empty" });
  };

  const loadWindow = async (
    window: GuideWindow,
    preferredChannelId: string | undefined,
    anchorMs: number,
  ) => {
    if (disposed) return;
    request?.abort();
    const nextRequest = new AbortController();
    request = nextRequest;
    publish({ ...snapshot, error: undefined, status: "loading" });

    try {
      const sourceGuide = await source.load(window, nextRequest.signal);
      if (disposed || nextRequest.signal.aborted || request !== nextRequest) return;

      full = layoutGuide(sourceGuide, now());
      settle(narrow(full), preferredChannelId, anchorMs);
    } catch (error) {
      if (disposed || nextRequest.signal.aborted || request !== nextRequest) return;
      publish({
        ...snapshot,
        error: error instanceof Error ? error.message : "Couldn't load the Guide.",
        status: "error",
      });
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
      );
    },
    refresh: async (preferredChannelId) => {
      const at = now();
      await loadWindow(
        resolveWindow(at),
        preferredChannelId ?? snapshot.selection?.channelId,
        snapshot.selection?.anchorMs ?? at,
      );
    },
    restrict: (channelIds) => {
      if (disposed) return;
      only = channelIds ? new Set(channelIds) : undefined;
      if (!full || (snapshot.status !== "ready" && snapshot.status !== "empty")) return;
      settle(narrow(full), snapshot.selection?.channelId, snapshot.selection?.anchorMs ?? now());
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
