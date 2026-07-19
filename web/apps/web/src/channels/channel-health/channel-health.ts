import type { ChannelDTO } from "@loomarr/api";
import type { ChannelHealth, OnAirState } from "@/components/loomarr";

// The API's `status` and the card's `health` are deliberately different vocabularies
// (channel-card.type.ts): status is Loomarr's lifecycle (building/live/drifted/detached),
// health is what an operator needs to see at a glance. The mapping is HERE, once, rather
// than inline in the page, because "is this channel OK?" must mean the same thing on the
// list, on the detail screen, and in any test that asserts it.
//
// The interesting case is `live` with unfilled slots: the channel IS on air (Tunarr plays
// flex rather than dead air, §9) but it is not yet what the operator asked for, because
// acquisitions are still landing. Calling that plain "healthy" would hide the backfill
// they are waiting on, so it reads as pending-slots until every slot has a real program.
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
    default:
      return channel.programCount < channel.slotCount ? "pending-slots" : "healthy";
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
      return "off"; // detached: Tunarr lost it
  }
};

export { channelHealth, channelOnAir };
