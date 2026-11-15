import {
  type GuideChannelLayout,
  type GuideHealthState,
  type GuideSelection,
  guideChannelState,
  guideWaitingForTitles,
} from "@loomarr/core/guide";
import { brandChroma, decadeFill, Surface, Text, type TextTone } from "@loomarr/design-system";
import { memo, type ReactNode, type RefObject } from "react";
import { Pressable, View } from "react-native";

import { ChannelIdent } from "../../../channel-ident";
import { Block, selectionOf, WaitingBlock } from "../guide-blocks";

const ROW = 56;

// The row chip names what is wrong or in progress; a healthy channel shows none. The mock's
// words where it has them, and the shipped web grid's for the two it doesn't draw.
const healthChip: Record<GuideHealthState, { label: string; tone: TextTone }> = {
  "pending-slots": { label: "Still downloading", tone: "info" },
  drift: { label: "Updating", tone: "signal" },
  paused: { label: "Paused", tone: "muted" },
  creating: { label: "Creating", tone: "muted" },
  error: { label: "Error", tone: "danger" },
};

const nowLine = (ratio: number) => (
  <View
    pointerEvents="none"
    style={{
      backgroundColor: brandChroma[5],
      bottom: 0,
      left: `${ratio * 100}%`,
      position: "absolute",
      top: 0,
      width: 2,
    }}
  />
);

// Memoised: the web virtualiser re-renders on every scroll frame, and a row that stays in view
// shouldn't render again.
const GuideRow = memo(
  ({
    channel,
    focusPending,
    focusTick,
    nowRatio,
    onOpenChannel,
    onOpenChannelInfo,
    onSelect,
    railWidth,
    renderRowMenu,
    renderPreview,
    tabStop,
    timelineWidth,
    timezone,
  }: {
    channel: GuideChannelLayout;
    focusPending: RefObject<boolean>;
    focusTick?: number;
    nowRatio?: number;
    onOpenChannel?: (channelId: string) => void;
    onOpenChannelInfo?: (channelId: string) => void;
    onSelect?: (selection: GuideSelection) => void;
    railWidth: number;
    renderRowMenu?: (channel: GuideChannelLayout["source"]) => ReactNode;
    renderPreview?: (selection: GuideSelection) => ReactNode;
    /** The block that is the grid's Tab stop, when it is in this row. */
    tabStop?: string;
    timelineWidth: number;
    timezone?: string;
  }) => {
    const { broadcast, health } = guideChannelState(channel.source);
    const chip = health ? healthChip[health] : undefined;
    const waiting = guideWaitingForTitles(channel.source);
    const inRow = channel.airings.some((airing) => airing.scheduleBlockId === tabStop);
    return (
      <Surface
        backgroundColor="transparent"
        borderBottomColor="guideRowRule"
        borderBottomWidth={1}
        borderWidth={0}
        flexDirection="row"
        height={ROW + 1}
        // No fade for a channel that is off the air, unlike the mock: at the mock's 0.55 its text
        // fails WCAG contrast (2.4-3.8:1, #1705). The grey dot and the "Paused" chip mark it.
      >
        <Surface
          alignItems="center"
          backgroundColor="surfaceCanvas"
          borderColor="borderDecorative"
          borderRightWidth={1}
          borderWidth={0}
          flexDirection="row"
          gap={9}
          paddingRight={4}
          width={railWidth}
        >
          {/* The channel's decade colour as the mock's 4px bar at the rail's edge (#2767), the one
              place the grid shows it; clear without a decade. The bar takes the rail's old
              12px inset, so the ident sits where it did. */}
          <Surface
            aria-hidden
            backgroundColor={decadeFill(channel.source.decade) ?? "transparent"}
            borderRadius="round"
            borderWidth={0}
            flexShrink={0}
            height={24}
            width={4}
          />
          <ChannelIdent name={channel.source.name} number={channel.source.number} />
          <Pressable
            accessibilityRole="button"
            // The channel's name opens the channel's page; its blocks tune it. Out of the Tab order:
            // a hundred rows must not mean a hundred more Tab stops.
            tabIndex={-1}
            onPress={() => onOpenChannelInfo?.(channel.source.channelId)}
            style={{ alignItems: "center", flex: 1, flexDirection: "row", gap: 9, minWidth: 0 }}
          >
            <Text textRole="guideNumber">{channel.source.number}</Text>
            <View style={{ alignItems: "flex-start", flex: 1, minWidth: 0 }}>
              {/* Two lines so a long name reads in full (#3410). Two 18-point lines and the chip's
                  16 points plus its 2-point gap are 54, inside the row's fixed 56, so the rail
                  never changes the row geometry the cells share. */}
              {/* Left-aligned by hand: the name sits inside a button, and a button centres its text
                  by default, which a two-line name makes plain. */}
              <Text numberOfLines={2} textAlign="left" textRole="cardLabel">
                {channel.source.name}
              </Text>
              {chip ? (
                <Text marginTop={2} numberOfLines={1} textAlign="left" textRole="label" tone={chip.tone}>
                  {chip.label}
                </Text>
              ) : null}
            </View>
          </Pressable>
          <Surface
            backgroundColor={
              broadcast === "live"
                ? "guideOnAir"
                : broadcast === "reconciling"
                  ? "guideReconciling"
                  : "actionDisabled"
            }
            borderRadius="round"
            borderWidth={0}
            height={8}
            width={8}
          />
          {/* The ⋯ slot: the page's channel menu until the maintainer mocks its contents. */}
          <View style={{ alignItems: "center", width: 28 }}>{renderRowMenu?.(channel.source)}</View>
        </Surface>
        <View style={{ flex: 1, height: ROW, overflow: "hidden", position: "relative" }}>
          {waiting ? (
            <WaitingBlock
              airings={channel.airings}
              focusable={inRow}
              focusPending={focusPending}
              focusTick={focusTick}
              label={waiting}
              onOpen={() => onOpenChannel?.(channel.source.channelId)}
              onSelect={onSelect}
              selected={inRow}
            />
          ) : (
            channel.airings.map((airing) => (
              <Block
                airing={airing}
                focusable={airing.scheduleBlockId === tabStop}
                focusPending={focusPending}
                focusTick={focusTick}
                key={airing.scheduleBlockId}
                offAir={health === "paused"}
                onOpen={() => onOpenChannel?.(channel.source.channelId)}
                onSelect={onSelect}
                preview={renderPreview?.(selectionOf(airing))}
                px={airing.widthRatio * timelineWidth}
                selected={airing.scheduleBlockId === tabStop}
                timezone={timezone}
              />
            ))
          )}
          {nowRatio === undefined ? null : nowLine(nowRatio)}
        </View>
      </Surface>
    );
  },
);

export { GuideRow, nowLine, ROW };
