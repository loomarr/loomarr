interface PolicyFieldSourceProps {
  // Whether the field holds a channel value rather than its inherit sentinel.
  overridden: boolean;
  // Writes the field's inherit sentinel.
  onReset: () => void;
  // The id of the field's visible label, which describes the Reset button.
  labelledBy?: string;
  overrideLabel?: string;
  defaultLabel?: string;
  hideResetWhenDefault?: boolean;
  className?: string;
}

export type { PolicyFieldSourceProps };
