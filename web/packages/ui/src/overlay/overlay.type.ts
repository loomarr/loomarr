import type { Density, DialogContentProps } from "@loomarr/design-system";
import type { ReactNode } from "react";

type OverlayAction = {
  disabled?: boolean;
  label: string;
  onPress: () => void;
  preferredFocus?: boolean;
  /** The current choice among several (a setting's stored value), drawn with Action's selected state. */
  selected?: boolean;
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
  /** `false` makes it one that must be answered: no close, and Back, Escape and the scrim do nothing. */
  dismissible?: boolean;
  onDismiss: () => void;
  /** Web only: where the modal portals, for a contained renderer such as the story gallery. */
  portalContainer?: DialogContentProps["portalContainer"];
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
