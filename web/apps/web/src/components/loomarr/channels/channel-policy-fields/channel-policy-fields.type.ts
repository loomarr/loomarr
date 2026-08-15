import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";

interface ChannelPolicyFieldsProps {
  policy: ChannelPolicy;
  // Controlled: the parent owns the policy and persists it. Every edit here produces a
  // NEW policy object with just that field merged in — `applied` (reconcile-owned, §9)
  // and every other section are always carried through untouched.
  onChange: (next: ChannelPolicy) => void;
  // `show` renders a subset so the Programming surface (§12) can split the fields across its
  // "What plays" (scope: audience ceiling + era) and "How it's ordered" (ordering + no-repeat)
  // blocks. Omitted = render all (the standalone use).
  show?: "scope" | "ordering";
  // The CHANNEL's playback strategy — not part of ChannelPolicy, but rendered here because
  // its only observable effect is as the fallback for `policy.ordering`: the Ordering select
  // has always offered "inherit the channel's Strategy" while nothing could show or set the
  // value being inherited. Both optional, so the standalone use (no channel in scope) is
  // unchanged and simply omits the control.
  strategy?: string;
  onStrategyChange?: (next: string) => void;
  className?: string;
}

export type { ChannelPolicyFieldsProps };
