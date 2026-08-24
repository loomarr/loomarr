import type { Density } from "@loomarr/design-system";
import type { ReactNode } from "react";

type OverlayAction = {
  disabled?: boolean;
  label: string;
  onPress: () => void;
  preferredFocus?: boolean;
  tone?: "danger" | "primary" | "secondary";
};

type OverlayContent = {
  actions?: readonly OverlayAction[];
  children?: ReactNode;
  density?: Density;
  description?: string;
  eyebrow?: string;
  title: string;
};

type ModalOverlayProps = OverlayContent & {
  dismissible?: boolean;
  onDismiss: () => void;
  reducedMotion?: boolean;
  visible: boolean;
};

type TransientOverlayProps = OverlayContent & {
  /** A positive duration dismisses the overlay unless it currently contains interaction. */
  autoDismissMs?: number;
  onDismiss: () => void;
  placement?: "bottom" | "top";
  reducedMotion?: boolean;
  visible: boolean;
};

export type { ModalOverlayProps, OverlayAction, TransientOverlayProps };
