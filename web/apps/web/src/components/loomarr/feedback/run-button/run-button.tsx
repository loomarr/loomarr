import { Loader2, Play } from "lucide-react";
import { Button } from "@/components/ui";
import type { RunButtonProps } from "./run-button.type";

// RunButton — "do this now" with visible, honest feedback that it is happening.
//
// ⚠ **The reason this is a component and not three inline props.** On the Tasks page a plain
// button driven by the server's own `running` flag showed NOTHING: `POST /run` returns 202 in
// milliseconds and the flag is true for only ~250ms, both of which close before the list
// refetch lands. Measured by sampling the button 120 times over 6 seconds and observing exactly
// one state — "Run now", enabled, for the entire run. Any surface with a "run it now" affordance
// over a queued backend hits the same thing, so the fix belongs here rather than being
// rediscovered per page.
//
// The caller owns `busy`; `useRunFeedback` is the hook that computes it correctly.
//
// ⚠ Progress is INDETERMINATE by design (§18.1). A job's Run returns only at the end, so any
// percentage would be invented — and a bar that reaches 90% and stops is a worse claim than a
// spinner that says only "running".
const RunButton = ({ busy, onRun, label, busyLabel, disabled, className, ...rest }: RunButtonProps) => (
  <Button
    variant="outline"
    size="sm"
    className={className}
    disabled={busy || disabled}
    // aria-busy is what tells a screen reader the control is mid-action; `disabled` alone says
    // "unavailable", which is a different claim.
    aria-busy={busy}
    onClick={onRun}
    {...rest}
  >
    {busy ? <Loader2 className="size-4 animate-spin" aria-hidden /> : <Play className="size-4" aria-hidden />}
    {busy ? (busyLabel ?? "Running…") : (label ?? "Run now")}
  </Button>
);

export { RunButton };
