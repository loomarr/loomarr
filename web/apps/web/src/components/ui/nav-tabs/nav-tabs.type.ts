import type { ReactNode } from "react";

interface NavTab {
  /** Identifies the tab, and matches `activeId`. Used for the panel's `aria-controls` target. */
  id: string;
  label: ReactNode;
  /**
   * Where this tab goes. Every tab is a real destination — see NavTabs for why.
   *
   * ⚠ Typed loosely rather than as the router's `LinkProps["to"]` because this package must not
   * depend on TanStack Router: the component library is consumed by Storybook and (later) a
   * native shell, and neither has it. The caller passes a route path and search params; the
   * `linkComponent` it also passes is what turns them into navigation.
   */
  to: string;
  /** Search params for the destination, e.g. `{ tab: "sources" }`. */
  search?: Record<string, unknown>;
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
   * Tints the count pill (`suggest`) while the count is above zero: this tab holds work that is
   * waiting on the viewer. Only the underline variant draws it — the v2 mock's Queue bar does.
   */
  attention?: boolean;
}

/** The shape of the router's Link, narrowed to what this component passes it. */
type LinkLike = (props: {
  to: string;
  search?: Record<string, unknown>;
  activeOptions?: { exact: boolean; includeSearch?: boolean };
  className?: string;
  children?: ReactNode;
  "aria-current"?: "page";
  "aria-controls"?: string;
  id?: string;
}) => ReactNode;

interface NavTabsProps {
  tabs: NavTab[];
  activeId: string;
  /**
   * The router's `Link`. Injected rather than imported so this stays router-agnostic — the
   * component owns the LOOK, the caller owns navigation.
   */
  linkComponent: LinkLike;
  /**
   * Names the bar for screen readers ("Filler sections"). Required rather than defaulted: a
   * generic label on a page with two tab bars tells a screen-reader user nothing.
   */
  label: string;
  /**
   * `pill` (default) is Settings' filled pill. `underline` is the v2 mock's Queue tab bar: a
   * transparent tab with a 2px underline in the active colour, sitting on a 1px rule. The two
   * exist because different screens' mocks draw different bars; pick the one the screen's mock
   * shows rather than restyling.
   */
  variant?: "pill" | "underline";
  className?: string;
}

export type { LinkLike, NavTab, NavTabsProps };
