import type { ChannelPolicy } from "@loomarr/api";

interface ChannelPolicyFieldsProps {
  policy: ChannelPolicy;
  // Controlled: the parent owns the policy and persists it. Every edit here produces a
  // NEW policy object with just that field merged in — `applied` (reconcile-owned, §9)
  // and every other section are always carried through untouched.
  onChange: (next: ChannelPolicy) => void;
  className?: string;
}

export type { ChannelPolicyFieldsProps };
