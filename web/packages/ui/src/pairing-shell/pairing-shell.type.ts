import type { PairingCredential, PairingSession } from "@loomarr/core/pairing";
import type { ServerDiscovery } from "@loomarr/core/server-discovery";
import type { Density } from "@loomarr/design-system";
import type { ReactNode } from "react";

/**
 * A phone's in-app browser sheet. With it, a touch device approves itself by opening the server's
 * `/pair` page instead of showing a QR code for a second device. The shell closes it once paired.
 */
type ApprovalBrowser = {
  /** Shown on the button: "Approve in Safari". */
  name: string;
  close(): void;
  open(url: string): void;
};

type PairingShellProps = {
  allowServerEntry?: boolean;
  approvalBrowser?: ApprovalBrowser;
  density: Density;
  discovery?: ServerDiscovery;
  discoveryForeground?: boolean;
  initialServerUrl?: string;
  renderPaired(credential: PairingCredential): ReactNode;
  session: PairingSession;
};

export type { ApprovalBrowser, PairingShellProps };
