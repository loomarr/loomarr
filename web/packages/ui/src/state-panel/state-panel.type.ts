import type { Density } from "@loomarr/design-system";
import type { ReactNode } from "react";

type StatePanelKind = "empty" | "error" | "loading" | "offline" | "permission";

type StatePanelAction = {
  label: string;
  onPress: () => void;
};

type StatePanelProps = {
  /** Exactly one recovery action keeps an interrupted journey decisive. */
  action?: StatePanelAction;
  /** Use the reduced vertical composition when the panel shares a bounded app shell. */
  compact?: boolean;
  description?: string;
  density?: Density;
  icon?: ReactNode;
  kind: StatePanelKind;
  metadata?: string;
  title: string;
};

export type { StatePanelAction, StatePanelKind, StatePanelProps };
