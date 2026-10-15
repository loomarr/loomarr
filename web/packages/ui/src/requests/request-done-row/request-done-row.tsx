import { Surface, Text } from "@loomarr/design-system";
import { Pressable } from "react-native";

import { requestedOn } from "../request-card";
import type { RequestDoneRowProps } from "./request-done-row.type";

// A finished request is a way to watch its channel, so the row is one target that does that: the
// channel's number and name over when it was asked for. The API stamps no ready time on a request (only
// when it was created and last updated), so the line says "Requested", never a "Ready" it can't back.
// One accessible element, so VoiceOver reads the row once.
const RequestDoneRow = ({ channel, entry, nowMs, onOpen }: RequestDoneRowProps) => {
  const { journey } = entry;
  const name = channel?.name ?? journey.intent.description;
  const when = requestedOn(journey.createdAt, nowMs);
  return (
    <Pressable
      accessibilityHint="Tunes this channel and opens Watching"
      accessibilityLabel={`${channel ? `Channel ${channel.number}, ` : ""}${name}. ${when}`}
      accessibilityRole="button"
      onPress={onOpen}
      style={{ minHeight: 48, width: "100%" }}
    >
      <Surface
        alignItems="center"
        flexDirection="row"
        gap="$control"
        level="raised"
        minHeight={48}
        padding="$control"
      >
        {channel ? (
          <Text density="touch" textRole="title" tone="secondary">
            {channel.number}
          </Text>
        ) : null}
        <Surface backgroundColor="$transparent" borderWidth={0} flex={1} gap="$inline">
          <Text density="touch" numberOfLines={2} textRole="title">
            {name}
          </Text>
          <Text density="touch" textRole="caption" tone="secondary">
            {when}
          </Text>
        </Surface>
      </Surface>
    </Pressable>
  );
};

export { RequestDoneRow };
