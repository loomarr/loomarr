interface ErrorDetailsProps {
  // The full failure text. Falsy renders nothing, so a caller can pass an optional field
  // straight through without guarding at every call site.
  message?: string;
  showLabel?: string; // default "Show error"
  hideLabel?: string; // default "Hide error"
  className?: string;
}

export type { ErrorDetailsProps };
