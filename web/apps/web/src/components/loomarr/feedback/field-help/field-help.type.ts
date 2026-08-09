interface FieldHelpProps {
  // The help text, shown in a tooltip on hover/focus of the (i) icon.
  children: string;
  // Names the field this help is about, for the trigger's accessible label
  // (e.g. "Ordering" → aria-label "About Ordering"). Screen-reader users reach the same
  // text a sighted user sees on hover.
  label: string;
  // Id of an element ALREADY holding this help text, when the consumer renders one.
  //
  // Base UI's tooltip is visual-only (no `aria-describedby`), so FieldHelp declares the
  // description itself — but `SettingField` already renders the doc in a hidden <p> and points the
  // CONTROL at it, which is the better anchor. Passing that id here reuses it instead of putting
  // the same prose in the DOM twice. Omit it and FieldHelp renders its own copy.
  describedById?: string;
  className?: string;
}

export type { FieldHelpProps };
