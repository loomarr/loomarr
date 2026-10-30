import { Action, type Density, playerChrome, Text } from "@loomarr/design-system";
import { View } from "react-native";

// The Playback info sheet's body (§9, mock "playback-info"; #2088 decision 13), opened from
// Settings. Everyone reads the Stream rows — what this browser is playing. An admin also reads the
// server's side of the channel and can copy the stream URL to open it in another player.

type InfoRow = readonly [string, string];

interface PlayerPlaybackInfoProps {
  /** The server's side, for an admin only: omitted for a member. */
  admin?: { copied: boolean; onCopyStreamUrl?: () => void; rows: readonly InfoRow[] };
  density: Density;
  /** `[label, value]`: the rung, resolution, codecs, bitrate, buffer and lag. */
  stream: readonly InfoRow[];
}

const SectionLabel = ({ density, label }: { density: Density; label: string }) => (
  <Text density={density} shout textRole="guideMeta" tone="secondary" tracking={0.16}>
    {label}
  </Text>
);

const InfoRows = ({
  density,
  label,
  rows,
}: {
  density: Density;
  label: string;
  rows: readonly InfoRow[];
}) => (
  <View aria-label={label} role="list">
    {rows.map(([key, value]) => (
      <View
        key={key}
        role="listitem"
        style={{
          borderBottomColor: playerChrome.glassRing,
          borderBottomWidth: 1,
          flexDirection: "row",
          gap: 16,
          justifyContent: "space-between",
          paddingVertical: 8,
        }}
      >
        <Text density={density} style={{ flexShrink: 0 }} textRole="compact" tone="secondary">
          {key}
        </Text>
        <Text
          density={density}
          style={{ flexShrink: 1, textAlign: "right" }}
          textRole="cardTime"
          tone="primary"
        >
          {value}
        </Text>
      </View>
    ))}
  </View>
);

const PlayerPlaybackInfo = ({ admin, density, stream }: PlayerPlaybackInfoProps) => (
  <View style={{ gap: 20 }}>
    <View style={{ gap: 6 }}>
      <SectionLabel density={density} label="Stream" />
      <InfoRows density={density} label="Stream" rows={stream} />
    </View>
    {admin ? (
      <View style={{ gap: 6 }}>
        <View style={{ alignItems: "center", flexDirection: "row", gap: 8 }}>
          <SectionLabel density={density} label="Admin" />
          <View
            style={{
              backgroundColor: playerChrome.signalWash,
              borderRadius: 4,
              paddingHorizontal: 6,
              paddingVertical: 2,
            }}
          >
            <Text density={density} shout strong textRole="guideMeta" tone="signal">
              Admin only
            </Text>
          </View>
        </View>
        <InfoRows density={density} label="Admin" rows={admin.rows} />
        {admin.onCopyStreamUrl ? (
          <Action
            density={density}
            icon={admin.copied ? "success" : "copy"}
            onPress={admin.onCopyStreamUrl}
            style={{ marginTop: 10 }}
            variant="outline"
          >
            {admin.copied ? "Copied" : "Copy stream URL"}
          </Action>
        ) : null}
      </View>
    ) : null}
  </View>
);

export type { PlayerPlaybackInfoProps };
export { PlayerPlaybackInfo };
