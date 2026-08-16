import type { Intent } from "@loomarr/api/models/intent";

// A one-click starter intent — the blank-page killer (§13). The stable id is navigation
// identity; the nested Intent is the exact request the preset promises. Keeping those
// concerns separate prevents a caller from accidentally handing off only the description.
interface ChannelTemplate {
  id: string;
  label: string;
  intent: Intent;
}

export type { ChannelTemplate };
