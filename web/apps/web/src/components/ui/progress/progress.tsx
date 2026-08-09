import { cn } from "@/lib";
import type { ProgressProps, ProgressTone } from "./progress.type";

// Progress — the determinate/indeterminate bar (§5.1c, Layer 1).
//
// ⚠ **A styled div carrying `role="progressbar"`, never a native `<progress>`** — the same
// measured finding already recorded for `<meter>` at `confidence-meter`. A native element's fill
// lives in `::-webkit-progress-*` pseudo-elements that Tailwind classes cannot reach, and
// `appearance-none` strips the platform rendering without supplying a replacement, so the bar
// draws the browser's own colour rather than a token. The explicit role plus the aria-value trio
// carries exactly what the native element would have contributed to assistive tech.
//
// ⚠ **No `aria-live` here, ever.** A queue of forty clips would mount forty live regions and read
// as the "chorus of live regions" `frontend-design.md` §5.3 forbids. A surface that needs to
// announce progress owns exactly ONE `role="status"` region carrying the most recent transition
// sentence — the announcement is the page's job, never the bar's.
//
// ⚠ The other three bars in the app (database-migration, restart-overlay, ai-model-settings) are
// sequenced onto this in the following slice rather than converted here, so the pipeline work and
// a four-surface refactor do not land in one diff. It is a scheduled migration, not an orphan.
const TONE: Record<ProgressTone, string> = {
  tune: "bg-tune",
  signal: "bg-signal",
  lock: "bg-lock",
  onair: "bg-onair",
};

const Progress = ({ value, label, tone = "tune", className }: ProgressProps) => {
  const determinate = value != null;
  const pct = determinate ? Math.max(0, Math.min(100, value)) : 100;

  return (
    <span
      role="progressbar"
      // ⚠ The value trio is spread CONDITIONALLY and as a unit. An indeterminate bar must carry
      // none of it — `aria-valuenow` alone is what turns "busy" into a false measurement, and
      // min/max without now describes a range nothing sits in.
      {...(determinate ? { "aria-valuenow": pct, "aria-valuemin": 0, "aria-valuemax": 100 } : {})}
      aria-label={label}
      className={cn("block h-1.5 w-full overflow-hidden rounded-full bg-static-800", className)}
    >
      <span
        className={cn(
          "block h-full rounded-full",
          TONE[tone],
          // Indeterminate reads as motion rather than as a full bar. ⚠ `motion-safe:` because the
          // pulse stops under prefers-reduced-motion (§2.4) — at which point the bar is a plain
          // dimmed track, which is why the accessible name is required rather than optional.
          !determinate && "opacity-60 motion-safe:animate-pulse",
        )}
        style={{ width: `${pct}%` }}
      />
    </span>
  );
};

export { Progress };
