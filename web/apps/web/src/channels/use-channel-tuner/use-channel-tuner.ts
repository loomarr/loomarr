import type { ChannelDTO } from "@loomarr/api";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
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

const afterNextPaint = (fn: () => void) => {
  if (typeof requestAnimationFrame !== "function") return;
  requestAnimationFrame(() => requestAnimationFrame(fn));
};

// useChannelTuner owns intent ordering, not media transport. The route mirrors its target, while
// pendingId keeps rapid key presses moving from the LAST REQUESTED channel even before React Router
// has committed the previous URL. That is the heart of latest-request-wins channel surfing.
const useChannelTuner = ({
  currentId,
  channels,
  nowNext,
  onTune,
}: UseChannelTunerOptions): UseChannelTuner => {
  const catalog = useMemo(() => surfableCatalog(channels), [channels]);
  const [request, setRequest] = useState<{ channel: ChannelDTO; attempt: ReturnType<typeof beginTune> }>();
  const pendingId = useRef(currentId);
  const requestedId = useRef<string | undefined>(undefined);

  useEffect(() => {
    // A navigation from outside this controller becomes the new base. Keep our request while the
    // route catches up to it; clear only when the URL genuinely names some other channel.
    pendingId.current = currentId;
    if (requestedId.current === currentId) return;
    requestedId.current = undefined;
    setRequest(undefined);
  }, [currentId]);

  const current = request?.channel ?? catalog.find((channel) => channel.id === currentId);
  const step = useCallback(
    (direction: TuneDirection) => {
      const target = adjacentChannel(catalog, pendingId.current, direction);
      if (!target || (catalog.length === 1 && target.id === pendingId.current)) return;
      const attempt = beginTune(true);
      pendingId.current = target.id;
      requestedId.current = target.id;
      setRequest({ channel: target, attempt });
      afterNextPaint(() => markTunePhase(attempt, "osd"));
      onTune(target);
    },
    [catalog, onTune],
  );

  const retry = useCallback(() => {
    if (!current) return;
    const attempt = beginTune(false);
    pendingId.current = current.id;
    requestedId.current = current.id;
    setRequest({ channel: current, attempt });
    afterNextPaint(() => markTunePhase(attempt, "osd"));
  }, [current]);

  const currentTitle = current
    ? nowNext.find((entry) => entry.channelId === current.id)?.now?.title
    : undefined;

  return {
    channel: current,
    currentTitle,
    attempt: request?.attempt,
    canSurf: catalog.length > 1,
    step,
    retry,
  };
};

export { adjacentChannel, surfableCatalog, useChannelTuner };
