import type { GuideHighlightDTO } from "@loomarr/api/models/guideHighlightDTO";

interface TonightProps {
  // GET /v1/guide/highlights, in airtime order.
  highlights: readonly GuideHighlightDTO[];
  // The guide's household timezone, so times read as the Guide shows them.
  timeZone?: string;
}

export type { TonightProps };
