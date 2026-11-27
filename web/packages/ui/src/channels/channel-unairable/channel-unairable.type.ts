/** Why a lineup title cannot air, as reconcile records it on the channel's policy. */
type UnairableReason =
  | "no_episodes_in_seasons"
  | "no_episodes_selected"
  | "not_in_library"
  | "out_of_scope"
  | "over_ceiling"
  | "unrated";

/** One lineup title that puts nothing on the channel, with why. */
interface UnairableTitle {
  key: string;
  reason: UnairableReason;
  title: string;
}

interface ChannelUnairableProps {
  /** The titles the last reconcile could not air, in lineup order. Nothing renders when empty. */
  titles: readonly UnairableTitle[];
}

export type { ChannelUnairableProps, UnairableReason, UnairableTitle };
