import type { ChannelPolicy } from "@loomarr/api";

interface ChannelAutoCurateProps {
  policy: ChannelPolicy;
  // Controlled, like ChannelPolicyFields: the parent owns the policy and persists it. Every
  // edit produces a NEW policy object with `autoCurate` merged in (or removed, for the
  // opt-out) and every other section carried through untouched.
  onChange: (next: ChannelPolicy) => void;
  // Whether this channel is intent-backed (`ChannelDTO.intentRef !== ""`). Re-curation
  // re-runs the channel's own intent job (programming-design.md §8.2), so a hand-made
  // channel has nothing to re-evaluate and the job skips it. Passed in rather than inferred
  // so the control can say *why* it is unavailable instead of silently accepting a setting
  // that will never fire.
  intentBacked?: boolean;
  className?: string;
}

export type { ChannelAutoCurateProps };
