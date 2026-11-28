import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import type { NowNextEntry } from "@loomarr/api/models/nowNextEntry";
import { formatProgrammeTitle } from "@loomarr/core/guide";
import type { OnAirState } from "@/components/loomarr/channels/on-air-indicator";

// `buildSchedule` marks the No schedule yet state, which offers an admin Build schedule (#2728).
type AirState = { dot: OnAirState; label: string; detail: string; buildSchedule?: boolean };

// headerAirLine is the line under the name (the web mock): a red dot and "On now: <title> · Nm
// left" while a programme airs, a grey dot and "Paused" for a paused channel. The mock draws only
// those two; any other state shows the page's own air label (Off air, Updating…) on the grey dot.
//
// `airing` is the programme as the timeline knows it, so it is named the way every other surface
// names it (#2748); the now/next entry carries only a title, and stands in until the timeline answers.
const headerAirLine = (
  air: AirState,
  now: NowNextEntry | undefined,
  nowMs: number,
  airing?: Pick<GuideAiring, "kind" | "series" | "title">,
): { onAir: boolean; text: string } => {
  if (air.dot === "live" && now && !now.gap && now.startMs <= nowMs && nowMs < now.stopMs) {
    const left = Math.max(1, Math.round((now.stopMs - nowMs) / 60_000));
    const title = airing ? formatProgrammeTitle(airing) : now.title;
    return { onAir: true, text: `On now: ${title} · ${left}m left` };
  }
  return { onAir: false, text: air.label };
};

// `scheduled` is whether now/next has anything for the channel: undefined until it answers, so the
// channel's own status stands until then.
const airStateOf = (ch: ChannelDTO, scheduled?: boolean): AirState => {
  // "Broadcasting" is backend-agnostic. Tunarr channels signal it with a pushed projection
  // (`tunarrId`); internal-playout channels — the DEFAULT backend, which never gets a
  // `tunarrId` — signal it with `inAppPlayable`, the server-resolved surfability truth
  // (§9.1 V57: effective backend plays internally AND the lifecycle is reconcilable). Keying
  // "On air" on `tunarrId` alone would tell every working internal-playout channel it was "Not on
  // air yet — connect Tunarr", contradicting reality at the first-channel "aha" moment.
  const pushed = Boolean(ch.tunarrId);
  const broadcasting = pushed || ch.inAppPlayable;
  if (ch.status === "detached") {
    return { dot: "off", label: "Off air", detail: "Loomarr no longer manages this channel." };
  }
  if (ch.status === "paused") {
    return {
      dot: "off",
      label: "Paused",
      detail: "Off air, but kept. Resume it below whenever you like.",
    };
  }
  if (ch.status === "drifted") {
    return {
      dot: "reconciling",
      label: "On air (catching up)",
      detail: "Something changed in your library. Loomarr is bringing the lineup back in line.",
    };
  }
  if (ch.status === "live" && broadcasting && scheduled === false) {
    // Live, but the guide has nothing for it now or next (#2728): On air here would sit beside an
    // empty guide row and "Programme information isn't available".
    return {
      dot: "off",
      label: "No schedule yet",
      detail: "Nothing is in the guide for this channel yet.",
      buildSchedule: true,
    };
  }
  if (ch.status === "live" && broadcasting) {
    return { dot: "live", label: "On air", detail: "Playing now in your TV guide." };
  }
  // building, or live-without-broadcasting (not yet reaching a viewer). An internal-playout
  // channel gets there on its own as it reconciles; a Tunarr-backed one once Tunarr connects.
  return {
    dot: "reconciling",
    label: "Not on air yet",
    detail: pushed
      ? "Getting ready to broadcast."
      : "It'll go live automatically once it's ready to broadcast.",
  };
};

export type { AirState };
export { airStateOf, headerAirLine };
