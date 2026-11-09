import { Surface, semanticColors, Text } from "@loomarr/design-system";
import { cloneElement, type ReactElement } from "react";

import type { WallTileProps } from "./wall-tile.type";

// The foot of the tile: dark enough under the words for any still.
const SCRIM = `linear-gradient(0deg, ${semanticColors.artwork.scrimStrong}, rgba(11, 12, 14, 0.4) 55%, rgba(11, 12, 14, 0))`;

// The channel number on a neutral block. #2767 gives it the channel's decade colour.
const NumberBlock = ({ number }: { number: string }) => (
  <Surface
    alignItems="center"
    backgroundColor="artworkScrimStrong"
    borderRadius={2}
    borderWidth={1}
    borderColor="borderDecorative"
    flexShrink={0}
    height={24}
    justifyContent="center"
    minWidth={32}
    paddingHorizontal={6}
  >
    <Text strong textRole="data">
      {number}
    </Text>
  </Surface>
);

// WallTile — one channel on Home's wall (#2764, Direction mock "Channel wall"): a 16:9 picture with
// the channel number and name along the top, what is on and a thin progress line along the foot. The
// tile is the control, so there is no filled button on it. Given a router `<Link>` it renders as that
// link, so middle-click and "open in new tab" work; otherwise it is a button.
const WallTile = ({
  art,
  channelName,
  compact = false,
  dim = false,
  detail,
  last = false,
  number,
  onClick,
  progressPercent,
  render,
  title,
}: WallTileProps) => {
  const host: ReactElement<Record<string, unknown>> = render ?? <button type="button" />;
  const own = host.props as { onClick?: (event: unknown) => void; style?: object };
  const pad = compact ? 8 : 12;

  return cloneElement(
    host,
    {
      ...own,
      onClick: (event: unknown) => {
        own.onClick?.(event);
        onClick?.();
      },
      style: {
        aspectRatio: "16 / 9",
        background: "var(--surfaceRaised)",
        border: last ? "none" : "1px solid var(--borderDecorative)",
        borderRadius: 12,
        boxShadow: last ? "0 0 0 2px var(--contentPrimary)" : undefined,
        boxSizing: "border-box",
        color: "var(--contentPrimary)",
        cursor: "pointer",
        display: "block",
        font: "inherit",
        isolation: "isolate",
        minWidth: 0,
        overflow: "hidden",
        padding: 0,
        position: "relative",
        textAlign: "left",
        textDecoration: "none",
        width: "100%",
        ...own.style,
      },
    },
    <>
      <Surface
        backgroundColor="transparent"
        borderWidth={0}
        bottom={0}
        left={0}
        opacity={dim ? 0.55 : 1}
        position="absolute"
        right={0}
        top={0}
      >
        {art}
      </Surface>
      <Surface
        backgroundColor="transparent"
        borderWidth={0}
        bottom={0}
        left={0}
        pointerEvents="none"
        position="absolute"
        right={0}
        style={{ backgroundImage: SCRIM }}
        top={0}
      />
      <Surface
        alignItems="center"
        backgroundColor="transparent"
        borderWidth={0}
        columnGap={8}
        flexDirection="row"
        left={0}
        padding={pad}
        position="absolute"
        right={0}
        top={0}
      >
        <NumberBlock number={number} />
        {!compact && (
          <Text as="span" numberOfLines={1} strong textRole="label">
            {channelName}
          </Text>
        )}
        {last && (
          <Surface
            backgroundColor="contentPrimary"
            borderRadius="round"
            borderWidth={0}
            marginLeft="auto"
            paddingHorizontal={8}
            paddingVertical={2}
          >
            <Text as="span" strong textRole="caption" tone="inverse">
              {compact ? "Last" : "You were here"}
            </Text>
          </Surface>
        )}
      </Surface>
      <Surface
        backgroundColor="transparent"
        borderWidth={0}
        bottom={0}
        gap={2}
        left={0}
        padding={pad}
        paddingBottom={compact ? 10 : 16}
        position="absolute"
        right={0}
      >
        <Text as="h3" numberOfLines={1} strong textRole={compact ? "label" : "cardTitle"}>
          {title}
        </Text>
        <Text as="span" numberOfLines={1} textRole="caption" tone="secondary">
          {compact ? [channelName, detail].filter(Boolean).join(" · ") : detail}
        </Text>
      </Surface>
      {progressPercent === undefined ? null : (
        <Surface
          backgroundColor="artworkProgressTrack"
          borderWidth={0}
          bottom={0}
          height={3}
          left={0}
          pointerEvents="none"
          position="absolute"
          right={0}
        >
          <Surface
            backgroundColor="contentPrimary"
            borderWidth={0}
            height={3}
            width={`${Math.max(0, Math.min(100, progressPercent))}%`}
          />
        </Surface>
      )}
    </>,
  );
};

export { WallTile };
