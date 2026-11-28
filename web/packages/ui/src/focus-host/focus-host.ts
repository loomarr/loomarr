import type { RingedPressableHandle } from "@loomarr/design-system";

/** Moves focus to a mounted control. A television takes a request for focus rather than `focus`. */
const focusHost = (host: RingedPressableHandle | null | undefined) => {
  const television = host as { requestTVFocus?: () => void } | null | undefined;
  if (television?.requestTVFocus) television.requestTVFocus();
  else host?.focus();
};

export { focusHost };
