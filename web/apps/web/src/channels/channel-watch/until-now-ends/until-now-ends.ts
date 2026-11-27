import type { GuideAiring } from "@loomarr/api/models/guideAiring";

// untilNowEnds is how long after the server's now the airing on now ends: the next moment time
// alone changes the channel's strip. Undefined when the server's now is unknown or nothing in the
// strip is still to end.
const untilNowEnds = (
  airings: readonly Pick<GuideAiring, "stopMs">[],
  serverNowMs: number | undefined,
): number | undefined => {
  if (serverNowMs === undefined) return undefined;
  let next: number | undefined;
  for (const { stopMs } of airings) {
    if (stopMs > serverNowMs && (next === undefined || stopMs < next)) next = stopMs;
  }
  return next === undefined ? undefined : next - serverNowMs;
};

export { untilNowEnds };
