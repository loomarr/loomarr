import type { CoverageDTO } from "@loomarr/api/models/coverageDTO";

interface CoverageMeterProps {
  // The server's answer, verbatim. ⚠ Typed as the generated DTO rather than re-declared:
  // the meter's entire claim is that it shows what the ladder computed, and a hand-mirrored
  // shape is the first step toward showing something else (contract 1:1).
  coverage: CoverageDTO;
  className?: string;
}

export type { CoverageMeterProps };
