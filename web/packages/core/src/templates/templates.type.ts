// A one-click starter intent — the blank-page killer (§13). A template is a partial
// intent (it must satisfy intentSchema once `description` is set), so the wizard's
// last step and the Suggest workspace can both prefill from the same data.
interface ChannelTemplate {
  id: string;
  label: string;
  description: string;
  era?: string;
  tone?: string;
}

export type { ChannelTemplate };
