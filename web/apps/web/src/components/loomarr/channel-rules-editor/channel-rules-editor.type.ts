import type { ChannelPolicy } from "@loomarr/api";

interface ChannelRulesEditorProps {
  policy: ChannelPolicy;
  // Controlled: the parent owns the policy and persists it (the same contract as
  // ChannelPolicyFields — every edit produces a NEW policy with just `rules` replaced,
  // everything else carried through untouched).
  onChange: (next: ChannelPolicy) => void;
  // The channel's own grounded lineup keys, so the `series:<key>` WHAT option lists real
  // titles rather than free text (programming-design §6.6: "intersected with the
  // channel's actually-grounded picks"). Optional — an empty/omitted list just means the
  // series option has nothing to offer yet.
  lineupKeys?: { key: string; title: string }[];
  className?: string;
}

export type { ChannelRulesEditorProps };
