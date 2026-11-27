import type { ReactElement, ReactNode } from "react";

import type { Density } from "../tokens";

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
  /**
   * `false` makes the dialog one that must be answered: Escape, Back, a press outside and the corner
   * close do nothing, and the corner close is not drawn. It is for a dialog that cannot close while
   * a call it started is in flight. Its answer is in its own actions, which close it by changing
   * `open`, so it needs a controlled `open`. Defaults to `true`.
   */
  dismissible?: boolean;
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
  /**
   * The dialog's name when it has no `DialogTitle` to show: a search surface whose own field says
   * what it is for (the command palette).
   */
  accessibilityLabel?: string;
  children: ReactNode;
  /**
   * Native only: the density the dialog is read at. A television's panel is wider (880) and roomier
   * (32 padding), and its title and description take the television's type; a phone's is the touch
   * type. The web adapter is the pointer's and ignores it. Defaults to `touch`.
   */
  density?: Density;
  /**
   * Web only: where focus goes when the dialog closes, for a dialog opened from a row that may no
   * longer be there (the detail panels' "Open details" buttons). Defaults to the trigger.
   */
  finalFocus?: { readonly current: PortalContainer | null };
  /**
   * `sectioned` draws the header and footer as ruled bands with their own padding, and leaves the
   * body between them to the caller (the people import list). `padded` pads the whole panel. A side
   * panel defaults to `sectioned`.
   */
  layout?: "padded" | "sectioned";
  /**
   * Web only: `side` docks the panel to the right edge at full height, for detail that sits beside
   * the list it came from (a clip, a source, a person). `top` holds a command surface near the top
   * of the screen, 576 wide at most, with no panel or corner close of its own: the content draws
   * its own surface, and Escape or a press outside closes it. `fill` covers the whole screen with
   * no panel or corner close of its own, for a phone's full-screen search: the content draws its
   * surface and its own close, since a phone has no Escape key. `bottom` docks a sheet to the bottom
   * edge across the full width, with no panel or corner close of its own: the content draws its own
   * surface (a `BottomSheet`) and its own close. Native always centres it.
   */
  placement?: "center" | "side" | "top" | "fill" | "bottom";
  /** Web only: where the dialog portals, for a contained renderer such as the story gallery. */
  portalContainer?: PortalContainer | null;
  /** The panel's widest: `default` 448, `form` 672, `reading` 768. */
  size?: "default" | "form" | "reading";
}

interface DialogSectionProps {
  children: ReactNode;
}

interface DialogFooterProps extends DialogSectionProps {
  /**
   * Its children stack, each the panel's full width, as an iOS alert's buttons do: what a phone
   * wants, since wrapping a row would size every button to the widest label.
   */
  stacked?: boolean;
  /** Its children spread from start to end, wrapping when they do not fit: one aside, one group. */
  spread?: boolean;
}

export type {
  DialogActionProps,
  DialogContentProps,
  DialogFooterProps,
  DialogProps,
  DialogSectionProps,
  PortalContainer,
};
