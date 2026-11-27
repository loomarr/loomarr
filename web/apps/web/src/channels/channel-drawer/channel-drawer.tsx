import { Action, DockedPanel, Field, PressableRow, Stack, Text, Tooltip } from "@loomarr/design-system";
import { useState } from "react";
import type { ChannelDrawerProps, DrawerBlock, DrawerChannel } from "./channel-drawer.type";

// A schedule block's fill: the signal hue for a filler pod, the text colour for a programme.
const POD_FILL = "color-mix(in srgb, var(--color-signal) 28%, transparent)";
const PROGRAMME_FILL = "color-mix(in srgb, var(--contentPrimary) 12%, transparent)";

// How many recent channels the drawer lists above All channels (the console mock).
const RECENT_ROWS = 3;

// A block's name only fits once it has this share of the strip (the console mock).
const LABEL_SHARE = 0.16;

const OFF_AIR_LINE: Record<NonNullable<DrawerChannel["offAir"]>, string> = {
  building: "building — not on air yet",
  paused: "off air",
};

// A block's name, when the block is wide enough to show it.
const shownBlockLabel = (block: DrawerBlock, total: number) =>
  block.label && total > 0 && block.minutes / total > LABEL_SHARE ? block.label : undefined;

const groupHeadingId = (header: string) => `channel-drawer-${header.toLowerCase().replaceAll(" ", "-")}`;

const channelNumber = (n: number) => String(n).padStart(2, "0");

const matches = (channel: DrawerChannel, query: string) =>
  `${channel.name} ${channel.now ?? ""} ${channelNumber(channel.number)}`.toLowerCase().includes(query);

// ChannelDrawer is the Watch page's channels drawer (#1659 W1), from the console mock: every
// channel with what's on now and the next two hours, favourites and recent channels first, a find
// box, and a star to pin a favourite. It sits inside the player's frame, over the right edge.
//
// Departures from the mock, both required by the redesign's accessibility floor: the star is a
// button beside the row's tune button, not inside it (no nested interactive elements), and the
// mock's tertiary grey (#5A6170, 2.7-3.2:1) becomes the muted text colour wherever it carries words.
const ChannelDrawer = ({
  channels,
  tunedId,
  favourites,
  recent,
  nowPercent,
  onTune,
  onFavourite,
  onClose,
}: ChannelDrawerProps) => {
  const [query, setQuery] = useState("");

  const q = query.trim().toLowerCase();
  const listed = q ? channels.filter((channel) => matches(channel, q)) : channels;
  const byId = new Map(listed.map((channel) => [channel.id, channel]));
  const starred = new Set(favourites);
  const groups: { header: string; rows: DrawerChannel[] }[] = [];
  if (q) {
    groups.push({ header: "MATCHES", rows: listed });
  } else {
    const favRows = favourites.flatMap((id) => byId.get(id) ?? []);
    const recentRows = recent
      .filter((id) => !starred.has(id))
      .slice(0, RECENT_ROWS)
      .flatMap((id) => byId.get(id) ?? []);
    if (favRows.length) groups.push({ header: "FAVORITES", rows: favRows });
    if (recentRows.length) groups.push({ header: "RECENT", rows: recentRows });
    groups.push({ header: "ALL CHANNELS", rows: listed });
  }

  return (
    // The design system's docked panel: docked in the player's frame, non-modal, role=dialog (so the
    // player's ↑/↓ leave its list alone), and it owns Escape.
    <DockedPanel
      title="Channels"
      closeLabel="Close channels"
      meta={
        <Text as="span" textRole="mono" tone="secondary" tracking={0.06}>
          NOW → NEXT 2H
        </Text>
      }
      onClose={onClose}
    >
      <Stack paddingHorizontal={10} paddingTop={10}>
        <Field
          autoFocus
          accessibilityLabel="Find a channel or show"
          inputMode="search"
          onChangeText={setQuery}
          placeholder="Find a channel or show…"
          value={query}
        />
      </Stack>
      <Stack flexBasis="0%" flexGrow={1} flexShrink={1} minHeight={0} overflowY="auto" padding={10}>
        {groups.map((group) => (
          <section key={group.header} aria-labelledby={groupHeadingId(group.header)}>
            <Stack paddingBottom={7} paddingHorizontal={4} paddingTop={6}>
              <Text
                as="h3"
                id={groupHeadingId(group.header)}
                textRole="mono"
                tone="secondary"
                tracking={0.14}
              >
                {group.header}
              </Text>
            </Stack>
            {group.rows.map((channel) => (
              <DrawerRow
                key={channel.id}
                channel={channel}
                tuned={channel.id === tunedId}
                favourite={starred.has(channel.id)}
                nowPercent={nowPercent}
                onTune={onTune}
                onFavourite={onFavourite}
              />
            ))}
          </section>
        ))}
      </Stack>
      <Stack
        borderTopColor="$borderDecorative"
        borderTopWidth={1}
        paddingHorizontal={18}
        paddingVertical={12}
      >
        <Text as="p" textRole="footnote">
          {q
            ? `${listed.length} of ${channels.length} channels · guide has the full week`
            : `${channels.length} channels · type a number to tune direct`}
        </Text>
      </Stack>
    </DockedPanel>
  );
};

const DrawerRow = ({
  channel,
  tuned,
  favourite,
  nowPercent,
  onTune,
  onFavourite,
}: {
  channel: DrawerChannel;
  tuned: boolean;
  favourite: boolean;
  nowPercent: number;
  onTune: (channelId: string) => void;
  onFavourite: (channelId: string, favourite: boolean) => void;
}) => {
  const total = channel.blocks.reduce((sum, block) => sum + block.minutes, 0);
  const line = channel.offAir ? OFF_AIR_LINE[channel.offAir] : channel.now;
  const shownLabels = channel.blocks.flatMap((block) => shownBlockLabel(block, total) ?? []);
  return (
    <Stack marginBottom={7} position="relative">
      <PressableRow
        // The row's text runs together when read as one name ("07Westerns▸ …"), so the name says it in
        // order. It holds every word the row shows (WCAG 2.5.3, label in name): the number and name,
        // the TUNED tag, the schedule strip's block names, what is on and the time left. Each piece of
        // text below ends in a space, so a tool that joins the row's text nodes reads separate words.
        accessibilityLabel={[
          `${channelNumber(channel.number)} ${channel.name}`,
          tuned ? "tuned" : undefined,
          ...shownLabels,
          line,
          channel.minutesLeft === undefined ? undefined : `${channel.minutesLeft}m left`,
        ]
          .filter(Boolean)
          .join(", ")}
        aria-current={tuned ? "true" : undefined}
        // The tuned row is neutral (#1909 Q2): elevated fill, the TUNED tag and `aria-current`.
        // Amber is left for the focus border, so a focused row is never confused with the tuned one.
        bordered
        onPress={() => onTune(channel.id)}
        selected={tuned}
      >
        <Stack gap="1.75">
          <Stack align="center" direction="row" gap="2" paddingRight={28} render="span">
            <Text as="span" minWidth={26} textRole="mono" tone="secondary">
              {channelNumber(channel.number)}{" "}
            </Text>
            <Text
              as="span"
              flex={1}
              minWidth={0}
              numberOfLines={1}
              strong
              textRole="cardLabel"
              tone="primary"
            >
              {channel.name}{" "}
            </Text>
            {tuned && (
              <Text as="span" textRole="mono" tone="secondary">
                TUNED{" "}
              </Text>
            )}
          </Stack>
          <Stack
            aria-hidden
            borderRadius={4}
            direction="row"
            gap="0.5"
            height={24}
            overflow="hidden"
            position="relative"
            render="span"
          >
            {channel.blocks.map((block) => (
              <Stack
                key={block.key}
                align="center"
                direction="row"
                flexBasis={0}
                flexGrow={block.minutes}
                flexShrink={1}
                minWidth={2}
                overflow="hidden"
                paddingHorizontal={6}
                render="span"
                style={{ backgroundColor: block.pod ? POD_FILL : PROGRAMME_FILL }}
              >
                {shownBlockLabel(block, total) && (
                  <Text as="span" minWidth={0} numberOfLines={1} textRole="cardMeta" tone="primary">
                    {block.label}{" "}
                  </Text>
                )}
              </Stack>
            ))}
            {channel.blocks.length > 0 && (
              <Stack
                bottom={0}
                position="absolute"
                render="span"
                style={{ backgroundColor: "var(--color-onair)", left: `${nowPercent}%` }}
                top={0}
                width={1.5}
              />
            )}
          </Stack>
          <Stack align="center" direction="row" gap="1.75" render="span">
            <Text as="span" flex={1} minWidth={0} numberOfLines={1} textRole="cardMeta" tone="secondary">
              {line ? `▸ ${line}` : ""}{" "}
            </Text>
            <Text as="span" flexShrink={0} textRole="mono" tone="secondary">
              {channel.minutesLeft === undefined ? "—" : `${channel.minutesLeft}m left`}
            </Text>
          </Stack>
        </Stack>
      </PressableRow>
      <Stack position="absolute" right={8} top={8}>
        <Tooltip content={favourite ? "Unpin favorite" : "Pin as favorite"}>
          {/* A pinned favourite is a pressed state, so it is foreground with the filled star, not amber. */}
          <Action
            aria-pressed={favourite}
            compact
            icon={favourite ? "starFilled" : "star"}
            layout="icon"
            onPress={() => onFavourite(channel.id, !favourite)}
            selected={favourite}
            variant="ghost"
          >
            {`Favorite ${channel.name}`}
          </Action>
        </Tooltip>
      </Stack>
    </Stack>
  );
};

export { ChannelDrawer };
