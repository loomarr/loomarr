interface ChannelCyclePreviewProps {
  channelId: string;
  // The channel's lineup as {key → show title}, so a slot (which carries only its provisioning
  // key + episode title) can show which SHOW it belongs to — "Bluey · S1E5 — Grandad" rather
  // than a bare "Grandad". Omitted ⇒ slots render episode-title-only (the prior behavior).
  lineupKeys?: { key: string; title: string }[];
  className?: string;
}

export type { ChannelCyclePreviewProps };
