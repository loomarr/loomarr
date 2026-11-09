import { Surface, Text } from "@loomarr/design-system";
import { Pressable } from "react-native";

import type { WallTileProps } from "./wall-tile.type";

// The native adapter: the tile as one pressable, its words under the number. `render` is a web router
// link and has no native meaning.
const WallTile = ({ channelName, detail, number, onClick, title }: WallTileProps) => (
  <Pressable accessibilityRole="button" onPress={onClick}>
    <Surface gap={2} padding={12}>
      <Text textRole="cardChannelNumber">{`${number} ${channelName}`}</Text>
      <Text numberOfLines={1} textRole="cardTitle">
        {title}
      </Text>
      {detail ? (
        <Text textRole="caption" tone="secondary">
          {detail}
        </Text>
      ) : null}
    </Surface>
  </Pressable>
);

export { WallTile };
