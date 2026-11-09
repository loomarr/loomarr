import { type Density, playerChrome, Text } from "@loomarr/design-system";
import { View } from "react-native";

import { PlaybarStill } from "../playbar-still";
import type { PlayerInfoData } from "../player-chrome.type";

// The Info sheet's body (§9, mock "info"; #2088 decision 9): the airing's still, its programme line,
// `year · rating · genres`, the synopsis, a time bar from its start to its end, then `Up next`.
// Nothing the server did not supply is drawn: a break has no still, facts or synopsis.

interface PlayerInfoProps {
  density: Density;
  /** Formats a wall-clock instant ("8:44 PM"). */
  formatTime: (ms: number) => string;
  info: PlayerInfoData;
}

const PlayerInfo = ({ density, formatTime, info }: PlayerInfoProps) => (
  <View>
    {info.stillUri ? (
      <View style={{ aspectRatio: 16 / 9, borderRadius: 6, marginBottom: 16, overflow: "hidden" }}>
        <PlaybarStill style={{ height: "100%", width: "100%" }} uri={info.stillUri} />
      </View>
    ) : null}
    <Text density={density} strong textRole="headline" tone="primary">
      {info.title}
    </Text>
    {info.facts ? (
      <Text density={density} style={{ marginTop: 4 }} textRole="compact" tone="secondary">
        {info.facts}
      </Text>
    ) : null}
    {info.synopsis ? (
      <Text density={density} style={{ marginTop: 12 }} textRole="body" tone="primary">
        {info.synopsis}
      </Text>
    ) : null}
    <View style={{ gap: 6, marginTop: 16 }}>
      <View style={{ backgroundColor: playerChrome.track, borderRadius: 3, height: 6, overflow: "hidden" }}>
        <View
          style={{
            backgroundColor: playerChrome.signal,
            height: "100%",
            width: `${Math.round(info.progress * 100)}%`,
          }}
        />
      </View>
      <View style={{ flexDirection: "row", justifyContent: "space-between" }}>
        <Text density={density} textRole="cardTime" tone="secondary">
          {formatTime(info.startMs)}
        </Text>
        {info.timeLabel ? (
          <Text density={density} textRole="cardTime" tone="secondary">
            {info.timeLabel}
          </Text>
        ) : null}
        <Text density={density} textRole="cardTime" tone="secondary">
          {formatTime(info.stopMs)}
        </Text>
      </View>
    </View>
    {info.upNext ? (
      <View
        style={{
          borderTopColor: playerChrome.hairline,
          borderTopWidth: 1,
          gap: 2,
          marginTop: 20,
          paddingTop: 16,
        }}
      >
        <Text density={density} shout textRole="mono" tone="info" tracking={0.16}>
          Up next
        </Text>
        <Text density={density} strong style={{ marginTop: 4 }} textRole="label" tone="primary">
          {`Up next · ${formatTime(info.upNext.startMs)} · ${info.upNext.name}`}
        </Text>
        {info.upNext.line ? (
          <Text density={density} textRole="cardMeta" tone="secondary">
            {info.upNext.line}
          </Text>
        ) : null}
      </View>
    ) : null}
  </View>
);

export type { PlayerInfoProps };
export { PlayerInfo };
