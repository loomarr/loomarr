interface PolicyFieldSourceProps {
  // Whether the field holds a channel value rather than its inherit sentinel.
  overridden: boolean;
  // Writes the field's inherit sentinel.
  onReset: () => void;
  // The field's visible label, which completes the Reset button's accessible name.
  label: string;
  overrideLabel?: string;
  defaultLabel?: string;
  hideResetWhenDefault?: boolean;
  className?: string;
}

export type { PolicyFieldSourceProps };
