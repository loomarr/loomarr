import type { PaletteScope, SearchResult } from "@loomarr/core/contracts";

// SearchResult / PaletteScope are FE view models (the ⌘K palette merges API candidates +
// channels + help — a superset of the generated SearchScope), so they live in core, not
// the generated client. The palette's prop interface (controlled + handlers) stays here.
interface SearchCommandProps {
  query: string;
  onQueryChange: (query: string) => void;
  results: SearchResult[];
  onSelect?: (result: SearchResult) => void;
  loading?: boolean;
  // Overrides the input's placeholder. Defaults to the ⌘K palette's wording, which is wrong
  // for the scoped consumers: the collections picker searches neither titles nor channels nor
  // help, so the default actively misdescribed what typing would do.
  placeholder?: string;
  // Called when Escape is pressed in the input. ⚠ Escape is NOT handled internally: the ⌘K
  // palette is a Dialog and the primitive owns the key there, so a built-in handler would close
  // twice. The SCOPED consumers each have a Cancel button, which is a POINTER affordance — a
  // keyboard user presses Escape and nothing happened. Opting in here binds it on the input itself
  // (already focusable, so no static-element handler) while leaving the palette single-close.
  //
  // ⚠ This said "the four scoped consumers" and there are FIVE, plus the palette — proposal-edit,
  // channel-collections-scope, channel-series-scope, channel-lineup-editor and filler-clip-list.
  // The comment was written when there were four and nothing counted them again.
  onEscape?: () => void;
  className?: string;
}

export type { PaletteScope, SearchCommandProps, SearchResult };
