import type { SetupCheck } from "@loomarr/api/models/setupCheck";
import type { ReactNode } from "react";

interface ConnectStepProps {
  // The setup/status check that reports whether the wiring took. The check, not the
  // click, is the source of truth (§6 "never silent").
  check?: SetupCheck;
  cta: string;
  onConnect: () => void;
  isPending?: boolean;
  error?: unknown;
  children?: ReactNode;
}

export type { ConnectStepProps };
