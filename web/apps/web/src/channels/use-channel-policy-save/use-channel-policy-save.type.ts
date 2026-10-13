import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";

interface ChannelPolicySave {
  // The policy the operator has asked for and the server has not confirmed yet, or undefined
  // when every edit is saved. Editors render THIS over the channel's saved policy, so an edit
  // made while an earlier one is in flight is built on the earlier one, not on the stale read.
  pending: ChannelPolicy | undefined;
  // Queues `next` (a whole policy, the PATCH's wholesale shape) for saving. Never sends two
  // PATCHes at once; edits made during a round trip coalesce into the next PATCH.
  save: (next: ChannelPolicy) => void;
  // True while any policy edit is unsaved.
  isSaving: boolean;
}

export type { ChannelPolicySave };
