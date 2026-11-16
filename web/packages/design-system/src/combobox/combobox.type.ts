import type { ReactNode } from "react";

import type { PortalContainer } from "../dialog/dialog.type";
import type { Density } from "../tokens";

type ComboboxOption = {
  /** A second, quieter line under the label (a region, a description). */
  description?: string;
  disabled?: boolean;
  label: string;
  /** Quiet content at the option's end: text (an item count, "Added"), or a badge beside it. */
  trailing?: ReactNode;
  value: string;
};

/** A labelled run of options. `id` is the group's stable key. */
type ComboboxGroup = {
  id: string;
  /** Names the group: drawn as its heading and read out as the group's name. */
  label: string;
  options: readonly ComboboxOption[];
};

type ComboboxProps = {
  /**
   * Options under headings, already narrowed, used in place of `options`. A group with no options is
   * not drawn. Arrow keys walk one sequence in the order drawn, so Down crosses a heading rather than
   * stopping at it.
   */
  groups?: readonly ComboboxGroup[];
  /**
   * The options to offer, already narrowed to what was typed. The combobox never filters: a server
   * search and a local list narrow differently, and only the caller knows which. Give this or
   * `groups`; `groups` wins when both are given.
   */
  options?: readonly ComboboxOption[];
  /** Ids of the help and validation lines the field is read with. */
  "aria-describedby"?: string;
  /** The id of a visible label, which then names the field in place of `label`. */
  "aria-labelledby"?: string;
  /**
   * Move the highlight to the first option as the list changes, so Enter picks it. Off, Enter submits
   * the form. `"always"` keeps the first option highlighted whether or not the user has typed, for a
   * list that is open before they do (inline).
   */
  autoHighlight?: boolean | "always";
  density?: Density;
  disabled?: boolean;
  /**
   * What to say when the list is open and there is nothing to pick ("No locations found."). Leave it
   * out and an empty list shows nothing at all.
   */
  emptyText?: string;
  /** What to say when the search failed. Read out as an alert. */
  errorText?: string;
  /** Web only: content after the field in the inline header (a close button, a key hint). */
  endAdornment?: ReactNode;
  /** Inline only: the panel fills its container and drops its card edge, the list taking the height. */
  fill?: boolean;
  /** The field's own id, for a caller's <label htmlFor>. */
  id?: string;
  /**
   * An always-open list under the field, drawn in place with no popup: a command palette inside a
   * dialog. The field is bare (no border: the panel is the surface) and the panel scrolls. Home and
   * End move the highlight to the first and last option here; in a floating list they keep moving
   * the text caret, as a text field's do. Escape does not clear the text.
   */
  inline?: boolean;
  /** The text in the field. The caller owns it, including the label of a chosen option. */
  inputValue: string;
  invalid?: boolean;
  /** The accessible name. Never printed: a visible label beside the field can name it instead. */
  label: string;
  /** Says a search is running: "Searching…" shows under any options. */
  loading?: boolean;
  /** The user edited the text. Not called when an option is picked: `onSelect` says that. */
  onInputValueChange: (value: string) => void;
  /**
   * Escape in the field. Inline it always fires; in a floating list the first Escape closes the
   * list and only the next one fires. Left out, Escape is not intercepted, so a dialog around the
   * field still closes on it. Web only: a phone has no Escape key.
   */
  onEscape?: () => void;
  /** An option was picked, by click, tap or Enter. */
  onSelect: (option: ComboboxOption) => void;
  /** Web only: the accessible name of the inline list, when it needs one beside the field's. */
  listLabel?: string;
  /** Web only: content before the field in the inline header (a search icon). */
  startAdornment?: ReactNode;
  placeholder?: string;
  /** Web only: where the list portals, for a contained renderer such as the story gallery. */
  portalContainer?: PortalContainer | null;
  /** Options that belong to an earlier search: drawn faded and not pickable until fresh ones arrive. */
  stale?: boolean;
  /** The value of the option that is the current choice: it carries a check. */
  value?: string | null;
};

export type { ComboboxGroup, ComboboxOption, ComboboxProps };
