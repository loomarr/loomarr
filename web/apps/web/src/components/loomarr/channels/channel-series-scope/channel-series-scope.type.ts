import type { ChannelPolicy } from "@loomarr/api";

interface ChannelSeriesScopeProps {
  policy: ChannelPolicy;
  // Controlled, like ChannelPolicyFields: the parent owns the policy and persists it.
  onChange: (next: ChannelPolicy) => void;
  className?: string;
}

export type { ChannelSeriesScopeProps };
