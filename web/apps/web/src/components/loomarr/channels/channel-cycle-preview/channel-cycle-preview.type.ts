import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";

interface ChannelCyclePreviewProps {
  channelId: string;
  // The channel's lineup as {key → show title}, so a slot (which carries only its provisioning
  // key + episode title) can show which SHOW it belongs to — "Bluey · S1E5 — Grandad" rather
  // than a bare "Grandad". Omitted ⇒ slots render episode-title-only (the prior behavior).
  lineupKeys?: { key: string; title: string }[];
  // The UNSAVED policy being edited above. When supplied, the panel previews THIS rather than
  // the saved channel — otherwise editing a rule and watching the pane beneath it show the
  // old schedule is worse than no preview, because it looks like the edit did nothing.
  //
  // ⚠ Omitted ⇒ byte-identical to the previous behaviour (the saved GET). The draft path is
  // additive; a caller that passes nothing cannot tell this prop exists.
  draftPolicy?: ChannelPolicy;
  className?: string;
}

export type { ChannelCyclePreviewProps };
