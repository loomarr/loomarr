import { useEffect, useRef, useState } from "react";
import { cn } from "@/lib/utils";
import type { ChannelDrawerProps, DrawerChannel } from "./channel-drawer.type";

// How many recent channels the drawer lists above All channels (the console mock).
const RECENT_ROWS = 3;

// A block's name only fits once it has this share of the strip (the console mock).
const LABEL_SHARE = 0.16;

const OFF_AIR_LINE: Record<NonNullable<DrawerChannel["offAir"]>, string> = {
  building: "building — not on air yet",
  paused: "off air",
};

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
    <section
      aria-labelledby="channel-drawer-title"
      className="border-static-700 bg-static-950/95 flex h-full w-[372px] max-w-full flex-col border-l backdrop-blur-md"
      onKeyDown={(e) => {
        if (e.key !== "Escape") return;
        e.stopPropagation();
        onClose();
      }}
    >
      <div className="border-static-800 flex items-center gap-2.5 border-b px-[18px] pt-4 pb-3">
        <h2 id="channel-drawer-title" className="text-sm font-semibold">
          Channels
        </h2>
        <span className="text-muted-foreground font-mono text-[11px] tracking-[.06em]">NOW → NEXT 2H</span>
        <button
          type="button"
          onClick={onClose}
          aria-label="Close channels"
          className="text-muted-foreground hover:text-static-0 focus-visible:ring-ring ml-auto flex size-6 items-center justify-center rounded focus-visible:ring-2 focus-visible:outline-none"
        >
          <span aria-hidden className="text-sm leading-none">
            ✕
          </span>
        </button>
      </div>
      <div className="px-2.5 pt-2.5">
        <input
          ref={findRef}
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Find a channel or show…"
          aria-label="Find a channel or show"
          className="border-border-control bg-static-950 text-foreground placeholder:text-muted-foreground focus:border-signal w-full rounded-lg border px-3 py-2 text-sm outline-none"
        />
      </div>
      <div className="flex-1 overflow-y-auto p-2.5">
        {groups.map((group) => (
          <section key={group.header} aria-labelledby={groupHeadingId(group.header)}>
            <h3
              id={groupHeadingId(group.header)}
              className="text-muted-foreground px-1 pt-1.5 pb-[7px] font-mono text-[11px] font-normal tracking-[.14em]"
            >
              {group.header}
            </h3>
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
      <p className="border-static-800 text-muted-foreground border-t px-[18px] py-3 font-mono text-[11px]">
        {q
          ? `${listed.length} of ${channels.length} channels · guide has the full week`
          : `${channels.length} channels · type a number to tune direct`}
      </p>
    </section>
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
  return (
    <div className="relative mb-[7px]">
      <button
        type="button"
        onClick={() => onTune(channel.id)}
        aria-current={tuned ? "true" : undefined}
        // The row's text runs together when read as one name ("07Westerns▸ …"); this says it in order,
        // starting with the visible number and name.
        aria-label={[
          `${channelNumber(channel.number)} ${channel.name}`,
          line,
          channel.minutesLeft === undefined ? undefined : `${channel.minutesLeft}m left`,
        ]
          .filter(Boolean)
          .join(", ")}
        // The tuned row is neutral (#1909 Q2): elevated fill, the TUNED tag and `aria-current`.
        // Amber is left for the focus ring, so a focused row is never confused with the tuned one.
        className={cn(
          "text-foreground focus-visible:ring-ring flex w-full flex-col gap-[7px] rounded-xl border px-3 py-[11px] text-left focus-visible:ring-2 focus-visible:outline-none",
          tuned ? "border-static-700 bg-static-800" : "border-static-800 hover:bg-static-900 bg-transparent",
        )}
      >
        <span className="flex items-center gap-[9px] pr-7">
          <span className="text-muted-foreground w-[26px] font-mono text-sm">
            {channelNumber(channel.number)}
          </span>
          <span className="min-w-0 flex-1 truncate text-sm font-semibold">{channel.name}</span>
          {tuned && <span className="text-muted-foreground font-mono text-[11px]">TUNED</span>}
        </span>
        <span aria-hidden className="relative flex h-6 gap-0.5 overflow-hidden rounded">
          {channel.blocks.map((block) => (
            <span
              key={block.key}
              className={cn(
                "flex min-w-0.5 items-center overflow-hidden px-1.5",
                block.pod ? "bg-signal/28" : "bg-foreground/12",
              )}
              style={{ flex: `${block.minutes} 1 0px` }}
            >
              {block.label && total > 0 && block.minutes / total > LABEL_SHARE && (
                <span className="text-foreground block min-w-0 truncate text-[11px]">{block.label}</span>
              )}
            </span>
          ))}
          {channel.blocks.length > 0 && (
            <span className="bg-onair absolute inset-y-0 w-[1.5px]" style={{ left: `${nowPercent}%` }} />
          )}
        </span>
        <span className="text-muted-foreground flex items-center gap-[7px] text-[11px]">
          <span className="min-w-0 truncate">{line ? `▸ ${line}` : ""}</span>
          <span className="ml-auto shrink-0 font-mono">
            {channel.minutesLeft === undefined ? "—" : `${channel.minutesLeft}m left`}
          </span>
        </span>
      </button>
      <button
        type="button"
        onClick={() => onFavourite(channel.id, !favourite)}
        aria-pressed={favourite}
        aria-label={`Favorite ${channel.name}`}
        title={favourite ? "Unpin favorite" : "Pin as favorite"}
        className={cn(
          // A pinned favourite is a pressed state, so it is foreground with the filled star, not amber.
          "hover:text-foreground focus-visible:ring-ring absolute top-2 right-2 flex size-6 items-center justify-center rounded focus-visible:ring-2 focus-visible:outline-none",
          favourite ? "text-foreground" : "text-muted-foreground",
        )}
      >
        <span aria-hidden className="text-sm leading-none">
          {favourite ? "★" : "☆"}
        </span>
      </button>
    </div>
  );
};

export { ChannelDrawer };
