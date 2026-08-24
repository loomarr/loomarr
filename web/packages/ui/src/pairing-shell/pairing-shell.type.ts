import type { PairingCredential, PairingSession } from "@loomarr/core/pairing";
import type { Density } from "@loomarr/design-system";
import type { ReactNode } from "react";

type PairingShellProps = {
  allowServerEntry?: boolean;
  density: Density;
  initialServerUrl?: string;
  renderPaired(credential: PairingCredential): ReactNode;
  session: PairingSession;
};

export type { PairingShellProps };
