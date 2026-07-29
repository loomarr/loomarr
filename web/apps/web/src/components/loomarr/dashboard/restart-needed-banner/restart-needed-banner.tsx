import { Button } from "@/components/ui";
import { cn } from "@/lib";
import type { RestartNeededBannerProps } from "./restart-needed-banner.type";

// RestartNeededBanner — "you changed a boot-time setting" (§9.2, config-design §3; v2 mock
// Dashboard).
//
// Almost every setting hot-applies, which is exactly why this needs saying: an operator
// who has learned that saving is enough will not guess that this one key is different.
// The banner names the key rather than saying "a setting changed", because "which one?"
// is the immediate next question and the server already knows the answer.
//
// ⚠ It does NOT restart on click. It routes to the control, where the consequences are
// stated — a one-click restart from a banner would skip the dialog that exists precisely
// because restarting is not free (§9.1).

const RestartNeededBanner = ({ pendingKeys, onGoToRestart, className }: RestartNeededBannerProps) => {
  // Nothing pending ⇒ no banner. Rendering an empty one would train operators to ignore
  // the space it occupies.
  if (pendingKeys.length === 0) return null;

  return (
    <div
      className={cn(
        "flex items-center gap-3.5 rounded-lg border border-signal/30 bg-signal-tint-15 p-3.5",
        className,
      )}
      role="status"
    >
      <span className="shrink-0 rounded bg-signal/15 px-2 py-0.5 font-mono text-[10px] text-signal uppercase">
        Restart needed
      </span>
      <p className="flex-1 text-sm leading-snug">
        You changed{" "}
        {/* Listing the keys, not counting them: an operator who edited two settings needs
            to know it is the database one that is waiting. */}
        <span className="font-mono text-xs">{pendingKeys.join(", ")}</span>. Loomarr is still running the old
        value until it restarts.
      </p>
      <Button type="button" onClick={onGoToRestart} className="shrink-0">
        Restart...
      </Button>
    </div>
  );
};

export { RestartNeededBanner };
