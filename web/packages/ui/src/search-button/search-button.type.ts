interface SearchButtonProps {
  // The shortcut that opens the same search, drawn at the trailing edge ("⌘K", "Ctrl K").
  hint?: string;
  // The same shortcut as `aria-keyshortcuts` names it ("Meta+K"), so assistive tech announces it.
  keyShortcuts?: string;
  onPress: () => void;
  // What the box says while it is empty ("Search…"). It is also the button's accessible name.
  placeholder: string;
}

export type { SearchButtonProps };
