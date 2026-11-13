import type { ComponentProps, ReactElement, ReactNode } from "react";
import type { Pressable } from "react-native";

import type { IconName } from "../../icons";
import type { Density } from "../../tokens";

/**
 * The four button looks (#2777): primary-filled (no `variant`: `tone` "primary", or "suggest" for the
 * AI accent), secondary-outline (`variant="outline"`), text-ghost (`variant="ghost"`) and icon-ghost
 * (`variant="ghost"` with `layout="icon"`). One filled button per screen, by convention: the screen's
 * one main action. Danger is a `tone` on the outline (`variant="outline" tone="danger"`), never a fill.
 * A `variant` is flat; without one the tile look draws a border and a focus ring sized for
 * remote/touch targets (TV and phone callers depend on that).
 */
type ActionVariant = "ghost" | "outline";

type ActionProps = Omit<ComponentProps<typeof Pressable>, "children" | "style"> & {
  /** A toggle's state when it is off as well as on; `selected` alone only ever announces pressed. */
  "aria-pressed"?: boolean;
  children: ReactNode;
  density?: Density;
  icon?: IconName;
  /**
   * `end` draws an inline icon after the label, for a glyph that points the way the action goes:
   * a pager's "Older" with its chevron toward older rows. The icon is decorative either way, so
   * the name stays the label alone.
   */
  iconPlacement?: "end" | "start";
  /**
   * Keep an inline label on one line by shrinking it to the room beside the icon, rather than
   * breaking a word that cannot fit (a 2×2 tile at an accessibility text size).
   */
  fitLabel?: boolean;
  /**
   * `stacked` puts the icon over a small label, on a tile with no outline until focused: the row of
   * player controls under a phone's picture (#1659 native mock 5e). Give it an icon.
   *
   * `icon` draws the icon alone on a square the density's target wide; the children, a string,
   * become its accessible name. For a row's Remove, a pane's close, a pager's arrows. Give it an icon.
   *
   * `text` draws the label alone as a line of hint-size text, with no box, control height or
   * padding: a field row's "Reset to default" beside its source badge, where a control-sized
   * button would wrap a narrow column. Focus is an outline, so the row does not move. Pair it with
   * `variant="ghost"`.
   */
  layout?: "icon" | "inline" | "stacked" | "text";
  /**
   * Web-only composition escape hatch (Base UI's `render` prop equivalent, #970 PR B checkpoint 3):
   * renders Action's resolved style and content onto this element instead of a `Pressable`, so a
   * button-like control can compose onto a router `Link`. The element's own children are replaced
   * by Action's `children`; its own event handlers, `style` and `className` are preserved and
   * merged with Action's. Ignored on native, where there is no comparable host-element swap.
   *
   * Composing this way loses the `Pressable` press dim (there is no gesture responder to drive
   * it), and `layout="stacked"`/`selected` are not meaningful on a transparent,
   * un-boxed renderer — this is for a plain `variant` button standing in for a link, not a tile.
   */
  render?: ReactElement<Record<string, unknown>>;
  selected?: boolean;
  /**
   * Web: a form's submit button. The Action renders onto a native `<button type="submit">` that
   * carries `disabled` itself, so Enter in a field cannot submit through a disabled one. Ignored
   * on native, and when `render` is given.
   */
  submit?: boolean;
  /**
   * With `layout="icon"`, a 24 px square holding the 16 px glyph, for a control that sits inside a
   * row or a chip: a remove cross, a drag handle, a help mark. It meets 2.5.8 (24 px) and nothing
   * more; a touch screen gets the full target through the caller's own hit slop.
   */
  compact?: boolean;
  style?: ComponentProps<typeof Pressable>["style"];
  /**
   * `success` is `danger`'s twin: the neutral tile in the success colour, for a yes beside a no. On
   * `variant="outline"`, `danger` draws the outline and its label in the danger colour. `suggest` is the
   * AI accent as the one filled button (dark text on the solid accent, which light text fails AA on).
   */
  tone?: "danger" | "primary" | "secondary" | "suggest" | "success";
  variant?: ActionVariant;
};

export type { ActionProps, ActionVariant };
