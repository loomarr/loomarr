import { cn } from "@/lib";
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
// print. The repo has been bitten before by hand-rolled widgets that looked right and had no
// keyboard path (SearchCommand's arrow keys, §12 drift `:757`).
const ErrorDetails = ({ message, showLabel, hideLabel, className }: ErrorDetailsProps) => {
  if (!message) return null;
  return (
    <details className={cn("group", className)}>
      <summary className="cursor-pointer list-none text-onair-300 text-xs hover:underline">
        <span className="group-open:hidden">{showLabel ?? "Show error"}</span>
        <span className="hidden group-open:inline">{hideLabel ?? "Hide error"}</span>
      </summary>
      {/* `whitespace-pre-wrap` keeps a multi-line error readable as written; `wrap-break-word`
          stops a long unbroken token (a URL, a stack frame) widening the whole container. */}
      <pre className="wrap-break-word mt-1 max-w-prose overflow-x-auto whitespace-pre-wrap rounded border border-onair/30 bg-onair/5 p-2 font-mono text-2xs text-onair-300">
        {message}
      </pre>
    </details>
  );
};

export { ErrorDetails };
