import { Surface, Text } from "@loomarr/design-system";
import { Pressable } from "react-native";

import type { HighlightRowProps } from "./list-row.type";

// The native adapter: one pressable row, the show and its reason stacked beside the time and channel
// number, read once as a single button. `render` is a web router link and has no native meaning.
const HighlightRow = ({ channelNumber, detail, onClick, reason, time, title }: HighlightRowProps) => (
  <Pressable accessibilityRole="button" onPress={onClick}>
    <Surface
      alignItems="center"
      backgroundColor="transparent"
      borderWidth={0}
      flexDirection="row"
      gap={12}
      paddingHorizontal={16}
      paddingVertical={12}
    >
      <Text textRole="data" width={80}>
        {time}
      </Text>
      <Text strong textRole="data" width={36}>
        {channelNumber}
      </Text>
      <Surface backgroundColor="transparent" borderWidth={0} flex={1} gap={2} minWidth={0}>
        <Text numberOfLines={1} strong textRole="body">
          {title}
          {detail != null && (
            <Text textRole="body" tone="secondary">
              {detail}
            </Text>
          )}
        </Text>
        {reason != null && (
          <Text textRole="caption" tone="secondary">
            {reason}
          </Text>
        )}
      </Surface>
    </Surface>
  </Pressable>
);

export { HighlightRow };
