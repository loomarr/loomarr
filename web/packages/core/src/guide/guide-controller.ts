import { getChannelGuideUrl } from "@loomarr/api/endpoints/channels";
import type { GuideOutputBody } from "@loomarr/api/models/guideOutputBody";
import { defaultGuideWindow, guideSelectionForChannel, layoutGuide } from "./guide";
import type { GuideController, GuideControllerOptions, GuideSourcePort } from "./guide-controller.type";

const createGuideSourcePort = (request: typeof globalThis.fetch): GuideSourcePort => ({
  load: async (window, signal) => {
    const response = await request(getChannelGuideUrl(window), { method: "GET", signal });
    if (!response.ok) throw new Error(`Couldn't load the Guide (${response.status}).`);
    return (await response.json()) as GuideOutputBody;
  },
});

const createGuideController = ({ now = Date.now, source }: GuideControllerOptions): GuideController => {
  let disposed = false;
  let request: AbortController | undefined;
  let snapshot: ReturnType<GuideController["getSnapshot"]> = { status: "loading" };
  const listeners = new Set<() => void>();
  const publish = (next: typeof snapshot) => {
    snapshot = next;
    for (const listener of listeners) listener();
  };

  return {
    dispose: () => {
      if (disposed) return;
      disposed = true;
      request?.abort();
      listeners.clear();
    },
    getSnapshot: () => snapshot,
    refresh: async (preferredChannelId) => {
      if (disposed) return;
      request?.abort();
      const nextRequest = new AbortController();
      request = nextRequest;
      publish({ ...snapshot, error: undefined, status: "loading" });
      const at = now();
      try {
        const sourceGuide = await source.load(defaultGuideWindow(at), nextRequest.signal);
        if (disposed || nextRequest.signal.aborted || request !== nextRequest) return;
        const layout = layoutGuide(sourceGuide, at);
        const requestedChannelId =
          preferredChannelId ?? snapshot.selection?.channelId ?? layout.channels[0]?.source.channelId;
        const requestedSelection = requestedChannelId
          ? guideSelectionForChannel(layout, requestedChannelId, snapshot.selection?.anchorMs ?? at)
          : undefined;
        const fallbackChannelId = layout.channels[0]?.source.channelId;
        const selection =
          requestedSelection ??
          (fallbackChannelId
            ? guideSelectionForChannel(layout, fallbackChannelId, snapshot.selection?.anchorMs ?? at)
            : undefined);
        publish({ layout, selection, status: layout.channels.length ? "ready" : "empty" });
      } catch (error) {
        if (disposed || nextRequest.signal.aborted || request !== nextRequest) return;
        publish({
          ...snapshot,
          error: error instanceof Error ? error.message : "Couldn't load the Guide.",
          status: "error",
        });
      }
    },
    select: (selection) => {
      if (disposed || snapshot.status !== "ready") return;
      publish({ ...snapshot, selection });
    },
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
};

export { createGuideController, createGuideSourcePort };
