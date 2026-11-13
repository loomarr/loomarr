import { Text } from "@loomarr/design-system";
import { overrideCount } from "@/lib/policy-overrides";
import { cn } from "@/lib/utils";
import type { ChannelDefaultsSummaryProps } from "./channel-defaults-summary.type";

// ChannelDefaultsSummary — the Programming tab's one line about defaults (#2741, #3412). A channel
// that changes nothing says so, and says a change here stays on this channel. A channel with
// changes draws nothing: each changed field marks itself.
const ChannelDefaultsSummary = ({ policy, className }: ChannelDefaultsSummaryProps) => {
  if (overrideCount(policy).overridden > 0) return null;
  return (
    <Text as="p" className={cn(className)} textRole="compact" tone="secondary">
      This channel uses Loomarr's defaults. Anything you change here applies to this channel only.
    </Text>
  );
};

export { ChannelDefaultsSummary };
