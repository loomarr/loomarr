import { overrideCount } from "@/lib/policy-overrides";
import { cn } from "@/lib/utils";
import type { ChannelDefaultsSummaryProps } from "./channel-defaults-summary.type";

// ChannelDefaultsSummary — the Programming tab's defaults summary (#1877 decision 1, Alt B): the
// split count, and plainly where the defaults come from. Almost every inherited value is a
// Loomarr built-in, so pointing at Settings for them would send the operator looking for knobs
// that do not exist.
const ChannelDefaultsSummary = ({ policy, className }: ChannelDefaultsSummaryProps) => {
  const { overridden, total } = overrideCount(policy);
  return (
    <div className={cn("rounded-lg border border-border bg-card p-4", className)}>
      <p className="text-sm">
        {overridden} of {total} fields are channel overrides. Most defaults here are Loomarr's built-in
        values, not a household setting — only Schedule horizon and Commercial-break frequency live on
        Settings → Channel defaults, and neither is on this page.
      </p>
    </div>
  );
};

export { ChannelDefaultsSummary };
