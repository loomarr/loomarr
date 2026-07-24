import type { PodEntryDTO, PodPoolDTOMatchLevel } from "@loomarr/api";

interface PodTimelineProps {
  // The generated preview shape, not a hand-written mirror: GET /v1/channels/{id}/pods
  // returns PodEntryDTO, which carries exactly what a segment renders.
  entries: PodEntryDTO[];
  // The §10 fallback ladder level, straight off the wire. This was a hand-written union
  // ("matched" | "fallback-widened" | "bumper-card-only") that had ALREADY drifted from
  // the four levels the assembler reports — the reason the API now enumerates it.
  matchLevel?: PodPoolDTOMatchLevel;
  era?: number;
  audience?: string;
  className?: string;
}

export type { PodTimelineProps };
