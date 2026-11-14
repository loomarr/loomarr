import type { IconProps } from "@loomarr/design-system";
import type { ReactNode } from "react";

interface NavTabLook {
  /** Identifies the tab, and matches `activeId`. */
  id: string;
  label: ReactNode;
  /**
   * A glyph drawn before the label from the `sm` width (640 px) up, and left out below it, where
   * the bar needs the room for the labels. Decorative: the label is the name.
   */
  icon?: IconProps["glyph"];
  /**
   * Shown as a pill beside the label.
   *
   * ⚠ OMIT it (rather than passing 0) for a tab that is not a countable queue. The difference
   * carries meaning: "0" says *zero things here*, absence says *counting does not apply*. A
   * Sources tab pinned at 0 reads as an empty list on an install with five sources.
   *
   * When present, the count and the list it labels must come from the same source — a tab
   * saying 3 above a list of 2 is worse than no count at all.
   */
  count?: number;
  /**
   * Tints the count pill (suggest) while the count is above zero: this tab holds work that is
   * waiting on the viewer. Both variants draw it, and on the pill bar it outranks the active tab's
   * own count colour.
   */
  attention?: boolean;
}

/** A tab that is a destination: a link (see NavTabs for why that is the default). */
interface NavTab extends NavTabLook {
  /**
   * Where this tab goes. Every tab is a real destination — see NavTabs for why.
   *
   * ⚠ Typed loosely rather than as the router's `LinkProps["to"]` because this package must not
   * depend on TanStack Router. The caller passes a route path and search params; the
   * `linkComponent` it also passes is what turns them into navigation.
   */
  to: string;
  /** Search params for the destination, e.g. `{ tab: "sources" }`. */
  search?: Record<string, unknown>;
}

/**
 * A tab that switches the view on the same page, where the view is state and not a URL (a page's
 * own Logs / Health / Processes views). It draws the same pill and is a button, so it has no `to`.
 */
type NavButtonTab = NavTabLook & { to?: never; search?: never };

/** The shape of the router's Link, narrowed to what this component passes it. */
type LinkLike = (props: {
  to: string;
  search?: Record<string, unknown>;
  activeOptions?: { exact: boolean; includeSearch?: boolean };
  style?: Record<string, unknown>;
  onMouseEnter?: () => void;
  onMouseLeave?: () => void;
  children?: ReactNode;
  "aria-current"?: "page";
  id?: string;
}) => ReactNode;

/**
 * How the tabs act. Either every tab is a destination and the router's `Link` carries it
 * (`linkComponent`), or the view is state and each tab is a button that reports its pick
 * (`onSelect`). Never both: a bar is one or the other.
 */
type NavTabsMode =
  | {
      tabs: readonly NavTab[];
      /**
       * The router's `Link`. Injected rather than imported so this stays router-agnostic — the
       * component owns the LOOK, the caller owns navigation.
       */
      linkComponent: LinkLike;
      onSelect?: never;
    }
  | {
      tabs: readonly NavButtonTab[];
      /**
       * Button mode: called with the picked tab's id. The current tab carries `aria-current="page"`,
       * as a link tab does, and pressing it again selects it again.
       */
      onSelect: (id: string) => void;
      linkComponent?: never;
    };

type NavTabsProps = NavTabsMode & {
  /**
   * The tab for the page being shown. `undefined` when the page is none of them, for example a
   * member who opened an admin-only channel section by URL: no tab is current then, because
   * `aria-current` on one would claim a page the viewer is not on (#1987).
   */
  activeId: string | undefined;
  /**
   * Names the bar for screen readers ("Filler sections"). Required rather than defaulted: a
   * generic label on a page with two tab bars tells a screen-reader user nothing.
   */
  label: string;
  /**
   * `pill` (default) is Settings' filled pill. `underline` is the v2 mock's Queue tab bar: a
   * transparent tab with a 2px underline in the active colour, sitting on a 1px rule. Pick the one
   * the screen's mock shows rather than restyling.
   */
  variant?: "pill" | "underline";
};

export type { LinkLike, NavButtonTab, NavTab, NavTabsProps };
