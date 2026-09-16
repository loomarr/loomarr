import type { PoolDTO } from "@loomarr/api/models/poolDTO";

interface PoolHealthProps {
  // The server's answer, verbatim. ⚠ Typed as the generated DTO rather than re-declared, for
  // the same reason CoverageMeter is: the strip's whole claim is that it reports what the
  // ladder computed, and a hand-mirrored shape is the first step toward reporting something
  // else (contract 1:1).
  pool: PoolDTO;
  className?: string;
}

export type { PoolHealthProps };
