import type { ChannelPolicy } from "@loomarr/api";

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
  // The CHANNEL's stored playback strategy — not part of ChannelPolicy, but named inside the
  // unset ordering option so the operator can see exactly what that fallback resolves to.
  // It remains read-only here: policy.ordering is the one operator-facing play-order knob.
  strategy?: string;
  className?: string;
}

export type { ChannelPolicyFieldsProps };
