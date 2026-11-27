import {
  Action,
  DockedPanel,
  PressableRow,
  Stack,
  Text,
  Tooltip,
  typography,
} from "@loomarr/design-system";
import { useEffect, useRef, useState } from "react";
import { cn } from "@/lib/utils";
import type { ChannelDrawerProps, DrawerBlock, DrawerChannel } from "./channel-drawer.type";

// The search box's type: the compact role's size at a line height of 1.25/0.875 of it, which sets
// the box's height (a text input is as tall as its line).
const SEARCH_TYPE = {
  fontSize: typography.pointer.compact.size,
  lineHeight: "calc(1.25 / 0.875)",
} as const;

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
  const findRef = useRef<HTMLInputElement>(null);
  useEffect(() => {
    findRef.current?.focus({ preventScroll: true });
  }, []);

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
      <div className="px-2.5 pt-2.5">
        <input
          ref={findRef}
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Find a channel or show…"
          aria-label="Find a channel or show"
          className="border-border-control bg-static-950 text-foreground placeholder:text-muted-foreground focus:border-signal w-full rounded-lg border px-3 py-2 outline-none"
          style={SEARCH_TYPE}
        />
      </div>
      <div className="flex-1 overflow-y-auto p-2.5">
        {groups.map((group) => (
          <section key={group.header} aria-labelledby={groupHeadingId(group.header)}>
            <Text
              as="h3"
              id={groupHeadingId(group.header)}
              className="px-1 pt-1.5 pb-[7px]"
              textRole="mono"
              tone="secondary"
              tracking={0.14}
            >
              {group.header}
            </Text>
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
      </div>
      <Text as="p" className="border-static-800 border-t px-[18px] py-3" textRole="footnote">
        {q
          ? `${listed.length} of ${channels.length} channels · guide has the full week`
          : `${channels.length} channels · type a number to tune direct`}
      </Text>
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
    <div className="relative mb-[7px]">
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
          <span className="flex items-center gap-[9px] pr-7">
            <Text as="span" className="w-[26px]" textRole="mono" tone="secondary">
              {channelNumber(channel.number)}{" "}
            </Text>
            <Text as="span" className="min-w-0 flex-1 truncate" strong textRole="cardLabel" tone="primary">
              {channel.name}{" "}
            </Text>
            {tuned && (
              <Text as="span" textRole="mono" tone="secondary">
                TUNED{" "}
              </Text>
            )}
          </span>
          <Stack
            aria-hidden
            className="relative h-6 overflow-hidden rounded"
            direction="row"
            gap="0.5"
            render="span"
          >
            {channel.blocks.map((block) => (
              <span
                key={block.key}
                className={cn(
                  "flex min-w-0.5 items-center overflow-hidden px-1.5",
                  block.pod ? "bg-signal/28" : "bg-foreground/12",
                )}
                style={{ flex: `${block.minutes} 1 0px` }}
              >
                {shownBlockLabel(block, total) && (
                  <Text as="span" className="block min-w-0 truncate" textRole="cardMeta" tone="primary">
                    {block.label}{" "}
                  </Text>
                )}
              </span>
            ))}
            {channel.blocks.length > 0 && (
              <span className="bg-onair absolute inset-y-0 w-[1.5px]" style={{ left: `${nowPercent}%` }} />
            )}
          </Stack>
          <Stack align="center" direction="row" gap="1.75" render="span">
            <Text as="span" className="min-w-0 truncate" textRole="cardMeta" tone="secondary">
              {line ? `▸ ${line}` : ""}{" "}
            </Text>
            <Text as="span" className="ml-auto shrink-0" textRole="mono" tone="secondary">
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
    </div>
  );
};

export { ChannelDrawer };
