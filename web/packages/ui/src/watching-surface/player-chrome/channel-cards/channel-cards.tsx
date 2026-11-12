import { type Density, decadeFillColor, playerChrome, Text } from "@loomarr/design-system";
import { Image, View } from "react-native";

import type { PlayerSurf, PlayerSurfChannel } from "../player-chrome.type";

// The channel cards (§9.2, variant J; the mock's "surfing" state): while channels are flipped, a row of
// cards takes the band's place. What is on is what a channel is chosen by, so each card has a picture
// of its own (the programme's, else a flat stand-in from its number) with its number block on it and
// the words under it, never words over a washed-out picture. The card just tuned is at full strength,
// ringed in amber; the rest sit back. Five on desktop and television, three on a phone.

interface ChannelCardsProps {
  density: Density;
  surf: PlayerSurf;
}

/** The widest the row runs, the band's own (§9.2). */
const MAX_WIDTH = 960;

/** Cards in the row: a phone has room for three. */
const countFor = (density: Density) => (density === "touch" ? 3 : 5);

/**
 * The cards on show: the tuned one centred where it can be, held against the ends of the lineup
 * rather than wrapping round it.
 */
const cardsAround = (channels: readonly PlayerSurfChannel[], tunedId: string, count: number) => {
  const at = Math.max(
    0,
    channels.findIndex((channel) => channel.id === tunedId),
  );
  const start = Math.min(Math.max(at - Math.floor(count / 2), 0), Math.max(channels.length - count, 0));
  return channels.slice(start, start + count);
};

/** A picture for a card without one: a flat tone from its number, as the film strip's stand-ins. */
const standInFor = (number: string) => `hsl(${(Number(number) * 47) % 360}, 24%, 17%)`;

const Card = ({ channel, density, lit }: { channel: PlayerSurfChannel; density: Density; lit: boolean }) => {
  const wide = density !== "touch";
  const fill = decadeFillColor(channel.decade);
  // The block's number keeps the band block's size on a television, where 30px digits overfill it.
  const blockText = density === "tv" ? "pointer" : density;
  return (
    <View
      role="listitem"
      style={{
        backgroundColor: lit ? playerChrome.surfaceHover : playerChrome.band,
        borderColor: lit ? playerChrome.signal : playerChrome.hairline,
        borderRadius: 12,
        borderWidth: lit ? 2 : 1,
        flex: 1,
        minWidth: 0,
        overflow: "hidden",
      }}
      testID={lit ? "channel-card-lit" : "channel-card"}
    >
      <View style={{ height: wide ? 76 : 48 }}>
        {/* Only the picture sits back: a number block dimmed with it falls below 4.5:1 on a decade
            colour, so the block keeps full strength on every card. */}
        <View
          style={{
            backgroundColor: channel.artUri ? playerChrome.surfaceRaised : standInFor(channel.number),
            bottom: 0,
            left: 0,
            opacity: lit ? 1 : 0.6,
            position: "absolute",
            right: 0,
            top: 0,
          }}
        >
          {channel.artUri ? (
            <Image
              accessibilityIgnoresInvertColors
              resizeMode="cover"
              source={{ uri: channel.artUri }}
              style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}
            />
          ) : null}
        </View>
        <View
          style={{
            alignItems: "center",
            backgroundColor: fill ?? (lit ? playerChrome.signal : playerChrome.surfaceRaised),
            borderRadius: 8,
            height: 32,
            justifyContent: "center",
            left: 8,
            minWidth: 40,
            paddingHorizontal: 8,
            position: "absolute",
            top: 8,
          }}
        >
          <Text
            density={blockText}
            textRole="guideNumber"
            tone={fill ? "onDecade" : lit ? "inverse" : "primary"}
          >
            {channel.number}
          </Text>
        </View>
      </View>
      <View
        style={{
          minWidth: 0,
          paddingBottom: wide ? 12 : 8,
          paddingHorizontal: wide ? 12 : 8,
          paddingTop: wide ? 10 : 6,
        }}
      >
        <Text
          density={density}
          numberOfLines={1}
          strong
          textRole={wide ? "label" : "cardMeta"}
          tone="primary"
        >
          {channel.name}
        </Text>
        <Text density={density} numberOfLines={1} textRole="cardMeta" tone={lit ? "signal" : "secondary"}>
          {lit ? "Watching now" : (channel.nowPlaying ?? "")}
        </Text>
      </View>
    </View>
  );
};

const ChannelCards = ({ density, surf }: ChannelCardsProps) => (
  <View
    aria-label="Channels"
    role="list"
    style={{ flexDirection: "row", gap: 8, maxWidth: MAX_WIDTH, minWidth: 0 }}
    testID="channel-cards"
  >
    {cardsAround(surf.channels, surf.tunedId, countFor(density)).map((channel) => (
      <Card channel={channel} density={density} key={channel.id} lit={channel.id === surf.tunedId} />
    ))}
  </View>
);

export type { ChannelCardsProps };
export { ChannelCards, cardsAround };
