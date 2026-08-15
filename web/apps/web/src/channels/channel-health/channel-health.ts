import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { ChannelHealth } from "@/components/loomarr/channels/channel-card";
import type { OnAirState } from "@/components/loomarr/channels/on-air-indicator";

// The API's `status` and the card's `health` are deliberately different vocabularies
// (channel-card.type.ts): status is Loomarr's lifecycle (building/live/drifted/detached),
// health is what an operator needs to see at a glance. The mapping is HERE, once, rather
// than inline in the page, because "is this channel OK?" must mean the same thing on the
// list, on the detail screen, and in any test that asserts it.
//
// The interesting case is `live` with titles still landing: the channel IS on air (Tunarr
// plays flex/filler rather than dead air, §9) but it is not yet what the operator asked
// for, because acquisitions are still in flight. Calling that plain "healthy" would hide the
// backfill they are waiting on, so it reads as pending-slots while any title is pending.
//
// Key on `pendingCount`, NOT `programCount < slotCount`: slotCount includes commercial-break
// gaps (§10), so a fully-acquired channel with ad breaks has programCount < slotCount forever
// and used to misread as "pending-slots" permanently. pendingCount counts only titles awaiting
// acquisition, so a healthy break-heavy channel is 0.
const channelHealth = (channel: ChannelDTO): ChannelHealth => {
  switch (channel.status) {
    case "building":
      return "creating";
    case "drifted":
      return "drift";
    case "detached":
      // Tunarr no longer has the channel Loomarr thinks it manages — the one state an
      // operator must act on, so it is an error rather than a soft warning.
      return "error";
    case "paused":
      // A deliberate operator pause is a real, calm, non-error state (unlike detached,
      // which is a fault). Its own health value so the card can render "Paused" plainly.
      return "paused";
    default:
      return channel.pendingCount > 0 ? "pending-slots" : "healthy";
  }
};

// On-air is about whether the viewer sees anything, which is a different question from
// health: a DRIFTED channel is still broadcasting (Tunarr keeps playing what it has), it
// just no longer matches what Loomarr intends — so it reads live here and drift there.
const channelOnAir = (channel: ChannelDTO): OnAirState => {
  switch (channel.status) {
    case "live":
    case "drifted":
      return "live";
    case "building":
      return "reconciling"; // no Tunarr channel yet; it is on its way
    default:
      return "off"; // detached (Tunarr lost it) or paused (operator took it off air)
  }
};

export { channelHealth, channelOnAir };
