import { type Density, playerChrome, Text } from "@loomarr/design-system";
import { View } from "react-native";

// The Keyboard shortcuts sheet's body (§9, mock "help"; the `?` key): each key in a keycap beside
// what it does. The app lists only the keys this player answers — no Subtitles before there are
// subtitles, no Picture-in-picture where the browser has none.

interface PlayerShortcutsProps {
  density: Density;
  /** `[keys, action]`: `["← / →", "Back / forward 10 s"]`. */
  rows: readonly (readonly [string, string])[];
}

const PlayerShortcuts = ({ density, rows }: PlayerShortcutsProps) => (
  <View aria-label="Keyboard shortcuts" role="list">
    {rows.map(([keys, action]) => (
      <View
        key={keys}
        role="listitem"
        style={{
          alignItems: "center",
          borderBottomColor: playerChrome.glassRing,
          borderBottomWidth: 1,
          flexDirection: "row",
          gap: 12,
          justifyContent: "space-between",
          paddingVertical: 8,
        }}
      >
        <View
          style={{
            backgroundColor: playerChrome.glassTop,
            borderColor: playerChrome.glassRing,
            borderRadius: 4,
            borderWidth: 1,
            paddingHorizontal: 8,
            paddingVertical: 2,
          }}
        >
          <Text density={density} textRole="cardTime" tone="primary">
            {keys}
          </Text>
        </View>
        <Text density={density} style={{ textAlign: "right" }} textRole="compact" tone="secondary">
          {action}
        </Text>
      </View>
    ))}
  </View>
);

export type { PlayerShortcutsProps };
export { PlayerShortcuts };
