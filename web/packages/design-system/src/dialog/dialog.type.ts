import type { ReactElement, ReactNode } from "react";

/**
 * A DOM element or shadow root, typed by shape so these shared props do not pull the DOM library
 * into the native packages that import the design system.
 */
interface PortalContainer {
  readonly nodeType: number;
}

interface DialogProps {
  children: ReactNode;
  defaultOpen?: boolean;
  /** Called with the next state: a trigger or close action, Escape, an outside press, Back. */
  onOpenChange?: (open: boolean) => void;
  open?: boolean;
}

/**
 * The caller's own control, which becomes the opener (the trigger) or a close action. On the web it
 * takes the primitive's click and ARIA props, so it must spread its props onto one element and
 * forward its ref; on native it takes `onPress`.
 */
interface DialogActionProps {
  children: ReactElement;
}

interface DialogContentProps {
  children: ReactNode;
  /**
   * `sectioned` draws the header and footer as ruled bands with their own padding, and leaves the
   * body between them to the caller (the people import list). `padded` pads the whole panel.
   */
  layout?: "padded" | "sectioned";
  /** Web only: where the dialog portals, for a contained renderer such as the story gallery. */
  portalContainer?: PortalContainer | null;
  /** The panel's widest: `default` 448, `form` 672, `reading` 768. */
  size?: "default" | "form" | "reading";
}

interface DialogSectionProps {
  children: ReactNode;
}

interface DialogFooterProps extends DialogSectionProps {
  /** Its children spread from start to end, wrapping when they do not fit: one aside, one group. */
  spread?: boolean;
}

export type { DialogActionProps, DialogContentProps, DialogFooterProps, DialogProps, DialogSectionProps };
