import type { UpdateChannelInputBody } from "@loomarr/api/models/updateChannelInputBody";

// One channel edit: any of the PATCH's fields except `revision`, which the writer owns.
type ChannelEdit = Omit<UpdateChannelInputBody, "revision" | "$schema">;

interface ChannelSaveOptions {
  // The toast once every queued edit is saved. The last edit queued names it; "Saved" otherwise.
  success?: string;
}

interface ChannelSave {
  // The edits the operator has asked for and the server has not confirmed yet, merged into one,
  // or undefined when everything is saved. Editors render the pending `policy` and `lineup` over
  // the saved channel, so an edit made while an earlier one is in flight builds on it.
  pending: ChannelEdit | undefined;
  // Queues `edit` for this channel. RESOLVES once a PATCH carrying it is saved; REJECTS with the
  // server's error after the writer has already shown it as a toast, so a caller that only fires
  // and forgets passes `ignoreFailure`, and a field that shows the reason inline awaits it.
  save: (edit: ChannelEdit, options?: ChannelSaveOptions) => Promise<void>;
  // True while any edit to this channel is unsaved.
  isSaving: boolean;
}

export type { ChannelEdit, ChannelSave, ChannelSaveOptions };
