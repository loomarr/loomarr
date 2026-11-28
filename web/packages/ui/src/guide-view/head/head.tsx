import type { GuideViewLook } from "@loomarr/core/guide-view";
import { Stack, Surface, Text } from "@loomarr/design-system";
import type { ReactNode } from "react";

import type { StripAiring } from "../../film-strip/series-runs";
import { ProgrammeFacts } from "../../programme-facts";
import type { GuideViewEntry, GuideViewLayout } from "../guide-view.type";

// The docked picture's size: a television keeps it small so that eight channels fit under it.
const PICTURE = {
  desktop: { height: 180, width: 320 },
  phone: { height: 190, width: "100%" },
  television: { height: 90, width: 160 },
} as const;

/** The airing a block shows now, or else its first. */
const airingOf = (look: GuideViewLook<GuideViewEntry>, nowMs: number): StripAiring | undefined => {
  const block = look.channel?.programmes.find((p) => p.id === look.programme?.id);
  return block?.airings.find((a) => a.startMs <= nowMs && nowMs < a.stopMs) ?? block?.airings[0];
};

const Looking = ({
  formatTime,
  layout,
  look,
  nowMs,
}: {
  formatTime: (ms: number) => string;
  layout: GuideViewLayout;
  look: GuideViewLook<GuideViewEntry>;
  nowMs: number;
}) => {
  const { channel, programme } = look;
  const heading = `Looking at ${channel?.name ?? ""}`;
  const airing = airingOf(look, nowMs);
  if (!programme || !airing) {
    return (
      <Stack gap="1">
        <Text textRole="label" tone="secondary">
          {heading}
        </Text>
        <Text textRole="title">Nothing scheduled</Text>
      </Stack>
    );
  }
  const when =
    programme.startMs > nowMs
      ? `${formatTime(programme.startMs)} to ${formatTime(programme.stopMs)}`
      : `on now until ${formatTime(programme.stopMs)}`;
  // A television's header is one line, beside a small picture.
  if (layout === "television") {
    return (
      <Text numberOfLines={1} textRole="cardLabel">
        {`${heading} · ${programme.title} · ${when}${airing.rating ? ` · ${airing.rating}` : ""}`}
      </Text>
    );
  }
  return (
    <Stack gap="1">
      <Text numberOfLines={1} textRole="label" tone="secondary">
        {`${heading} · ${when}`}
      </Text>
      <ProgrammeFacts
        airing={airing}
        formatTime={formatTime}
        nowMs={nowMs}
        startMs={programme.startMs}
        stopMs={programme.stopMs}
      />
    </Stack>
  );
};

/**
 * What the cursor is looking at, beside the docked picture. The picture keeps its own caption:
 * looking is not playing.
 */
const Head = ({
  formatTime,
  layout,
  look,
  lowerThird,
  nowMs,
  picture,
  watched,
}: {
  formatTime: (ms: number) => string;
  layout: GuideViewLayout;
  look: GuideViewLook<GuideViewEntry> | undefined;
  lowerThird: ReactNode;
  nowMs: number;
  picture: ReactNode;
  watched: GuideViewEntry | undefined;
}) => {
  const phone = layout === "phone";
  const words = !look ? null : look.playing ? (
    lowerThird
  ) : (
    <Looking formatTime={formatTime} layout={layout} look={look} nowMs={nowMs} />
  );
  const described = (
    <Stack aria-label="Looking" flex={phone ? 0 : 1} minWidth={0} role="group">
      {words}
    </Stack>
  );
  return (
    <Stack align={phone ? "stretch" : "start"} direction={phone ? "column" : "row"} gap="4">
      {phone ? null : described}
      <Stack flexShrink={0} gap="1" width={PICTURE[layout].width}>
        <Surface
          backgroundColor="surfaceElevated"
          borderRadius={6}
          borderWidth={0}
          height={PICTURE[layout].height}
          overflow="hidden"
        >
          {picture}
        </Surface>
        {watched ? (
          <Text numberOfLines={1} textRole="label" tone="secondary">
            {`Playing ${watched.number} ${watched.name}`}
          </Text>
        ) : null}
      </Stack>
      {phone ? described : null}
    </Stack>
  );
};

export { Head };
