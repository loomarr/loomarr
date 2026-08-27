import { FocusSurface, ProgressTrack, ScrollFrame, Surface, Text } from "@loomarr/design-system";
import { Pressable } from "react-native";

import type { SurfChannelData, SurfGroupKind, SurfRailProps } from "./surf-rail.type";

const TvSurfChannel = ({
  channel,
  current,
  groupLabel,
  onFocus,
  onTune,
  preferredFocus,
  register,
  selected,
}: {
  channel: SurfChannelData;
  current: boolean;
  groupLabel: SurfGroupKind;
  onFocus: () => void;
  onTune: () => void;
  preferredFocus: boolean;
  register: SurfRailProps["focusRegistry"];
  selected: boolean;
}) => (
  <Pressable
    accessibilityLabel={`${groupLabel}, channel ${channel.channelNumber}, ${channel.channelName}`}
    accessibilityRole="button"
    accessibilityState={{ selected }}
    hasTVPreferredFocus={preferredFocus}
    onFocus={onFocus}
    onPress={onTune}
    ref={(handle) => register?.register({ channelId: channel.id, group: groupLabel }, handle)}
  >
    <FocusSurface focused={selected} gap="$inline" paddingHorizontal="$control" paddingVertical="$inline">
      <Surface
        alignItems="center"
        backgroundColor="$transparent"
        borderWidth={0}
        flexDirection="row"
        gap="$control"
      >
        <Text density="tv" textRole="channelNumber" tone={selected ? undefined : "muted"}>
          {channel.channelNumber.padStart(2, "0")}
        </Text>
        <Text density="tv" flex={1} numberOfLines={1} textRole="body">
          {channel.channelName}
        </Text>
        {current ? (
          <Surface
            backgroundColor="$stateSuccess"
            borderRadius="$round"
            borderWidth={0}
            height={8}
            width={8}
          />
        ) : null}
      </Surface>
      {selected ? (
        <>
          <Surface
            alignItems="center"
            backgroundColor="$transparent"
            borderWidth={0}
            flexDirection="row"
            gap="$inline"
            paddingLeft={64}
          >
            <Text density="tv" flex={1} numberOfLines={1} textRole="metadata">
              {channel.now?.title ?? "Nothing scheduled"}
            </Text>
            {channel.now?.remainingLabel ? (
              <Text density="tv" textRole="metadata" tone="muted">
                {channel.now.remainingLabel}
              </Text>
            ) : null}
          </Surface>
          <Surface backgroundColor="$transparent" borderWidth={0} paddingLeft={64}>
            <ProgressTrack percent={channel.now?.progressPercent ?? 0} width="100%" />
          </Surface>
        </>
      ) : null}
    </FocusSurface>
  </Pressable>
);

const TvSurfRail = ({
  clientName = "Loomarr",
  clientVersion,
  currentChannelId,
  focusRegistry,
  groups,
  onFocusSelection,
  onTune,
  selection,
  serverVersion,
}: SurfRailProps) => {
  const selectable = groups.flatMap((group) =>
    group.channels.map((channel) => ({ channel, group: group.kind, groupLabel: group.label })),
  );
  const selectedIndex = Math.max(
    0,
    selectable.findIndex(
      ({ channel, group }) => selection.channelId === channel.id && selection.group === group,
    ),
  );

  return (
    <Surface
      accessibilityLabel="Channel surfer"
      backgroundColor="$transparent"
      borderRadius={0}
      borderWidth={0}
      flex={1}
    >
      <Surface
        backgroundColor="$surfaceOverlay"
        borderRadius={0}
        borderWidth={0}
        bottom={0}
        left={0}
        paddingBottom={48}
        paddingLeft={48}
        paddingRight="$control"
        paddingTop={48}
        position="absolute"
        top={0}
        width="44%"
      >
        <ScrollFrame density="tv">
          {groups.map((group) => (
            <Surface backgroundColor="$transparent" borderWidth={0} gap="$inline" key={group.kind}>
              <Text density="tv" textRole="metadata" tone="muted">
                {`${group.label.toUpperCase()} · ${group.channels.length}`}
              </Text>
              {group.channels.length === 0 ? (
                <Text density="tv" textRole="metadata" tone="muted">
                  {group.kind === "favourites" ? "No favourites yet" : "No recent channels yet"}
                </Text>
              ) : (
                group.channels.map((channel) => {
                  const selected = selection.group === group.kind && selection.channelId === channel.id;
                  return (
                    <TvSurfChannel
                      channel={channel}
                      current={channel.id === currentChannelId}
                      groupLabel={group.kind}
                      key={`${group.kind}-${channel.id}`}
                      onFocus={() => onFocusSelection({ channelId: channel.id, group: group.kind })}
                      onTune={() => onTune(channel.id)}
                      preferredFocus={selected}
                      register={focusRegistry}
                      selected={selected}
                    />
                  );
                })
              )}
            </Surface>
          ))}
        </ScrollFrame>
        <Text density="tv" textRole="metadata" tone="muted">
          {`${selectedIndex + 1} of ${selectable.length} · ▲▼ browse`}
        </Text>
        <Text density="tv" numberOfLines={1} textRole="metadata" tone="muted">
          {`${clientName} ${clientVersion} · Server ${serverVersion ?? "unavailable"}`}
        </Text>
      </Surface>
      <Surface bottom={48} level="overlay" padding="$control" position="absolute" right={48}>
        <Text density="tv" textRole="metadata" tone="muted">
          OK tune · BACK cancel
        </Text>
      </Surface>
    </Surface>
  );
};

export { TvSurfRail };
