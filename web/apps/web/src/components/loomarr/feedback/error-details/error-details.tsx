import { Text } from "@loomarr/design-system";
import { cn } from "@/lib/utils";
import type { ErrorDetailsProps } from "./error-details.type";

// ErrorDetails — a failure message that OPENS UP rather than being clipped to one line.
//
// Extracted from the Tasks page, where the lesson was concrete: that page is where an operator
// diagnoses a failing integration, and a truncated error is the one piece of information they
// came for. Any surface listing things that can fail (tasks, acquisitions, channels, filler
// sync) wants the same shape.
//
// ⚠ Collapsed BY DEFAULT, deliberately. One failing row in a list must not push every other
// row down the page — the operator scanning for "what is broken" needs the list to stay a list.
//
// ⚠ A native `<details>`, not a hand-rolled disclosure. It is keyboard-operable and announced
// as an expandable region by screen readers with no ARIA of our own, and it survives a page
// print. A hand-rolled widget can look right and have no keyboard path (SearchCommand's arrow
// keys, §12 drift `:757`).
const ErrorDetails = ({ message, showLabel, hideLabel, className }: ErrorDetailsProps) => {
  if (!message) return null;
  return (
    <details className={cn("group", className)}>
      <Text as="summary" className="cursor-pointer list-none hover:underline" textRole="hint" tone="danger">
        <span className="group-open:hidden">{showLabel ?? "Show error"}</span>
        <span className="hidden group-open:inline">{hideLabel ?? "Hide error"}</span>
      </Text>
      {/* `whitespace-pre-wrap` keeps a multi-line error readable as written; `wrap-break-word`
          stops a long unbroken token (a URL, a stack frame) widening the whole container. */}
      <Text
        as="pre"
        className="border-onair/30 bg-onair/5 mt-1 max-w-prose overflow-x-auto rounded border p-2 wrap-break-word whitespace-pre-wrap"
        textRole="mono"
        tone="danger"
      >
        {message}
      </Text>
    </details>
  );
};

export { ErrorDetails };
