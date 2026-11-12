import type { PortalContainer } from "../dialog/dialog.type";
import type { Density } from "../tokens";

type ComboboxOption = {
  /** A second, quieter line under the label (a region, a description). */
  description?: string;
  disabled?: boolean;
  label: string;
  /** Quiet text at the option's end (an item count, "Added"). */
  trailing?: string;
  value: string;
};

type ComboboxProps = {
  /** Ids of the help and validation lines the field is read with. */
  "aria-describedby"?: string;
  /** The id of a visible label, which then names the field in place of `label`. */
  "aria-labelledby"?: string;
  /** Move the highlight to the first option as the list changes, so Enter picks it. Off, Enter submits the form. */
  autoHighlight?: boolean;
  density?: Density;
  disabled?: boolean;
  /**
   * What to say when the list is open and there is nothing to pick ("No locations found."). Leave it
   * out and an empty list shows nothing at all.
   */
  emptyText?: string;
  /** What to say when the search failed. Read out as an alert. */
  errorText?: string;
  /** The field's own id, for a caller's <label htmlFor>. */
  id?: string;
  /** The text in the field. The caller owns it, including the label of a chosen option. */
  inputValue: string;
  invalid?: boolean;
  /** The accessible name. Never printed: a visible label beside the field can name it instead. */
  label: string;
  /** Says a search is running: "Searching…" shows under any options. */
  loading?: boolean;
  /** The user edited the text. Not called when an option is picked: `onSelect` says that. */
  onInputValueChange: (value: string) => void;
  /** An option was picked, by click, tap or Enter. */
  onSelect: (option: ComboboxOption) => void;
  /**
   * The options to offer, already narrowed to what was typed. The combobox never filters: a server
   * search and a local list narrow differently, and only the caller knows which.
   */
  options: readonly ComboboxOption[];
  placeholder?: string;
  /** Web only: where the list portals, for a contained renderer such as the story gallery. */
  portalContainer?: PortalContainer | null;
  /** Options that belong to an earlier search: drawn faded and not pickable until fresh ones arrive. */
  stale?: boolean;
  /** The value of the option that is the current choice: it carries a check. */
  value?: string | null;
};

export type { ComboboxOption, ComboboxProps };
