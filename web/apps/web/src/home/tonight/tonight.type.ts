import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";
import type { GuideHighlightDTO } from "@loomarr/api/models/guideHighlightDTO";

interface TonightProps {
  // GET /v1/guide: every channel with its airings. Each channel gets one row, for the next real
  // programme after `nowMs` (#2763); a channel with only filler ahead gets none.
  channels: readonly GuideChannelTimeline[];
  nowMs: number;
  // GET /v1/guide/highlights: the reason a programme is worth knowing about, when it has one.
  highlights: readonly GuideHighlightDTO[];
  // The guide's household timezone, so times read as the Guide shows them.
  timeZone?: string;
}

export type { TonightProps };
