import { Icon, icons, Surface, Text } from "@loomarr/design-system";
import { Pressable } from "react-native";

import type { RequestsBackProps } from "./requests-back.type";

// iPhone's navigation-bar back control: a chevron and the parent's name, no border or fill, at the top
// left, in the content colour (amber is for focus and the screen's primary action). The Pressable is
// 44 pt tall (and at least 44 wide) with a little reach to the left, so the glyph sits on the screen's
// gutter while the whole corner is the target.
const RequestsBack = ({ onPress }: RequestsBackProps) => (
  <Pressable
    accessibilityLabel="Back to Requests"
    accessibilityRole="button"
    hitSlop={{ left: 8, right: 8 }}
    onPress={onPress}
    style={{ alignSelf: "flex-start", justifyContent: "center", minHeight: 44, minWidth: 44 }}
  >
    <Surface
      alignItems="center"
      backgroundColor="$transparent"
      borderWidth={0}
      flexDirection="row"
      gap="$inline"
      marginLeft={-4}
    >
      <Icon decorative glyph={icons.back} size="control" tone="content" />
      <Text density="touch" textRole="body" tone="primary">
        Requests
      </Text>
    </Surface>
  </Pressable>
);

export { RequestsBack };
