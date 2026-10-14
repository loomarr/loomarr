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
  description?: string;
  density?: Density;
  icon?: ReactNode;
  kind: StatePanelKind;
  title: string;
};

export type { StatePanelAction, StatePanelKind, StatePanelProps };
