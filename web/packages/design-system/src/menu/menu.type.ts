import type { ReactElement, ReactNode } from "react";

import type { PortalContainer } from "../dialog/dialog.type";

interface MenuProps {
  children: ReactNode;
  /** Opens on first render, for a gallery story; a real menu opens from its trigger. */
  defaultOpen?: boolean;
  /** Told when the menu opens or closes, so a caller can reset state it armed inside the menu. */
  onOpenChange?: (open: boolean) => void;
}

interface MenuTriggerProps {
  /** The one element that opens the menu. It carries its own accessible name and look. */
  children: ReactElement;
}

interface MenuContentProps {
  children: ReactNode;
  side?: "bottom" | "left" | "right" | "top";
  align?: "center" | "end" | "start";
  /** Web only: where the panel portals, for a contained renderer such as the story gallery. */
  portalContainer?: PortalContainer | null;
  /** A fixed panel width; without it the panel sizes to its widest row. */
  width?: number;
}

interface MenuActionProps {
  children: ReactNode;
  onSelect: () => void;
  /** Drawn before the label, decorative: the label names the action. */
  icon?: ReactNode;
  disabled?: boolean;
  /** `false` keeps the menu open, for an item that arms a confirmation inside it. */
  closeOnSelect?: boolean;
  tone?: "danger";
}

export type { MenuActionProps, MenuContentProps, MenuProps, MenuTriggerProps };
