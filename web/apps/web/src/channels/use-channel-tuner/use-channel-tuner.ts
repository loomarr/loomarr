import type { ChannelDTO } from "@loomarr/api";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { type WarmedChannel, warmChannel as warmAdjacentChannel } from "../channel-warmer";
import { beginTune, markTunePhase } from "../tuner-timing";
import type { TuneDirection, UseChannelTuner, UseChannelTunerOptions } from "./use-channel-tuner.type";

// surfableCatalog consumes the SERVER'S effective-backend truth. Sorting has a stable id tie-break
// so even a corrupt duplicate channel number cannot make Up/Down order vary between renders.
const surfableCatalog = (channels: ChannelDTO[]): ChannelDTO[] =>
  channels
    .filter((channel) => channel.inAppPlayable)
    .sort((a, b) => a.number - b.number || a.id.localeCompare(b.id));

const adjacentChannel = (
  channels: ChannelDTO[],
  currentId: string,
  direction: TuneDirection,
): ChannelDTO | undefined => {
  if (channels.length === 0) return undefined;
  const current = channels.findIndex((channel) => channel.id === currentId);
  // A deep link to a channel that just became unsurfable joins at the nearest end rather than
  // producing an invalid index. Up enters the first channel; Down wraps to the last.
  if (current < 0) return direction > 0 ? channels[0] : channels.at(-1);
  return channels[(current + direction + channels.length) % channels.length];
};

const acrossNextPaint = (beforePaint: () => void, afterPaint: () => void) => {
  let beforeRan = false;
  let afterRan = false;
  const before = () => {
    if (beforeRan) return;
    beforeRan = true;
    beforePaint();
  };
  const after = () => {
    if (afterRan) return;
    afterRan = true;
    clearTimeout(fallback);
    afterPaint();
  };

  // A hidden/throttled Firefox document can stop delivering animation frames altogether. The
  // acknowledgement state has already been committed by the click, so give it one short paint
  // opportunity, then guarantee the tune intent advances even when rAF never arrives. Both paths
  // are idempotent because a late frame must not navigate a second time.
  const fallback = setTimeout(() => {
    before();
    after();
  }, 50);
  if (typeof requestAnimationFrame !== "function") return;
  requestAnimationFrame(() => {
    before();
    requestAnimationFrame(after);
  });
};

// useChannelTuner owns intent ordering, not media transport. The route mirrors its target, while
// pendingId keeps rapid key presses moving from the LAST REQUESTED channel even before React Router
// has committed the previous URL. That is the heart of latest-request-wins channel surfing.
const useChannelTuner = ({
  currentId,
  channels,
  nowNext,
  onTune,
  warmChannel = warmAdjacentChannel,
}: UseChannelTunerOptions): UseChannelTuner => {
  const catalog = useMemo(() => surfableCatalog(channels), [channels]);
  const [request, setRequest] = useState<{
    channel: ChannelDTO;
    attempt: ReturnType<typeof beginTune>;
    phase: "acknowledging" | "tuning";
  }>();
  const [activeId, setActiveId] = useState(currentId);
  const pendingId = useRef(currentId);
  const requestedId = useRef<string | undefined>(undefined);
  const latestAttemptId = useRef<number | undefined>(undefined);
  const warmed = useRef(new Map<string, WarmedChannel>());

  useEffect(() => {
    // A navigation from outside this controller becomes the new base. Keep our request while the
    // route catches up to it; clear only when the URL genuinely names some other channel.
    pendingId.current = currentId;
    if (requestedId.current === currentId) return;
    requestedId.current = undefined;
    setActiveId(currentId);
    setRequest(undefined);
  }, [currentId]);

  const current = catalog.find((channel) => channel.id === activeId);

  useEffect(() => {
    if (!current) return;
    const controller = new AbortController();
    const neighbors = [adjacentChannel(catalog, current.id, -1), adjacentChannel(catalog, current.id, 1)]
      .filter((channel): channel is ChannelDTO => Boolean(channel && channel.id !== current.id))
      .filter((channel, index, all) => all.findIndex((candidate) => candidate.id === channel.id) === index);
    const keep = new Set([current.id, ...neighbors.map((channel) => channel.id)]);
    for (const id of warmed.current.keys()) {
      if (!keep.has(id)) warmed.current.delete(id);
    }
    for (const neighbor of neighbors) {
      const cached = warmed.current.get(neighbor.id);
      if (cached && cached.expiresAt > Date.now() + 60_000) continue;
      void warmChannel(neighbor.id, controller.signal)
        .then((result) => {
          if (result && !controller.signal.aborted) warmed.current.set(neighbor.id, result);
        })
        .catch(() => {
          // Warming is speculative. A real tune still mints and attaches normally.
        });
    }
    return () => controller.abort();
  }, [catalog, current, warmChannel]);

  const step = useCallback(
    (direction: TuneDirection) => {
      const target = adjacentChannel(catalog, pendingId.current, direction);
      if (!target || (catalog.length === 1 && target.id === pendingId.current)) return;
      const warm = warmed.current.get(target.id);
      const attempt = beginTune(true, warm?.warmed, warm?.url);
      latestAttemptId.current = attempt.id;
      pendingId.current = target.id;
      requestedId.current = target.id;
      setRequest({ channel: target, attempt, phase: "acknowledging" });
      acrossNextPaint(
        () => {
          if (latestAttemptId.current !== attempt.id) return;
          markTunePhase(attempt, "osd");
        },
        () => {
          if (latestAttemptId.current !== attempt.id) return;
          setActiveId(target.id);
          setRequest((candidate) =>
            candidate?.attempt.id === attempt.id ? { ...candidate, phase: "tuning" } : candidate,
          );
          onTune(target);
        },
      );
    },
    [catalog, onTune],
  );

  const retry = useCallback(() => {
    if (!current) return;
    const warm = warmed.current.get(current.id);
    const attempt = beginTune(false, warm?.warmed, warm?.url);
    latestAttemptId.current = attempt.id;
    pendingId.current = current.id;
    requestedId.current = current.id;
    setRequest({ channel: current, attempt, phase: "acknowledging" });
    acrossNextPaint(
      () => {
        if (latestAttemptId.current !== attempt.id) return;
        markTunePhase(attempt, "osd");
      },
      () => {
        if (latestAttemptId.current !== attempt.id) return;
        setRequest((candidate) =>
          candidate?.attempt.id === attempt.id ? { ...candidate, phase: "tuning" } : candidate,
        );
      },
    );
  }, [current]);

  const titledChannel = request?.channel ?? current;
  const currentTitle = titledChannel
    ? nowNext.find((entry) => entry.channelId === titledChannel.id)?.now?.title
    : undefined;

  return {
    channel: current,
    requestedChannel: request?.channel,
    currentTitle,
    attempt: request?.phase === "tuning" ? request.attempt : undefined,
    acknowledging: request?.phase === "acknowledging",
    canSurf: catalog.length > 1,
    step,
    retry,
  };
};

export { adjacentChannel, surfableCatalog, useChannelTuner };
