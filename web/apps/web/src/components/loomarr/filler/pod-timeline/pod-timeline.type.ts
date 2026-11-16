import type { PodEntryDTO } from "@loomarr/api/models/podEntryDTO";
import type { PodPoolDTOMatchLevel } from "@loomarr/api/models/podPoolDTOMatchLevel";

interface PodTimelineProps {
  // The generated preview shape, not a hand-written mirror: GET /v1/channels/{id}/pods
  // returns PodEntryDTO, which carries exactly what a segment renders.
  entries: PodEntryDTO[];
  // The §10 fallback ladder level, straight off the wire: the API enumerates the four levels
  // the assembler reports, because a hand-written union here drifts from them.
  matchLevel?: PodPoolDTOMatchLevel;
  era?: number;
  audience?: string;
  className?: string;
  onPreview?: (entry: PodEntryDTO) => void;
}

export type { PodTimelineProps };
