interface FieldHelpProps {
  // The help text, shown in a tooltip on hover/focus of the (i) icon.
  children: string;
  // Names the field this help is about, for the trigger's accessible label
  // (e.g. "Ordering" → aria-label "About Ordering"). Screen-reader users reach the same
  // text a sighted user sees on hover.
  label: string;
  className?: string;
}

export type { FieldHelpProps };
