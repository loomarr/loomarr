import type { PoolDTO } from "@loomarr/api";

interface PoolHealthProps {
  // The server's answer, verbatim. ⚠ Typed as the generated DTO rather than re-declared, for
  // the same reason CoverageMeter is: the strip's whole claim is that it reports what the
  // ladder computed, and a hand-mirrored shape is the first step toward reporting something
  // else (contract 1:1).
  pool: PoolDTO;
  // Called when the operator asks Loomarr to plan a pull. Absent for a non-admin, who can read
  // catalog health but cannot start an acquisition.
  onProposePull?: () => void;
  // Whether a pull proposal is in flight, so the button can say so rather than looking inert.
  proposing?: boolean;
  className?: string;
}

export type { PoolHealthProps };
