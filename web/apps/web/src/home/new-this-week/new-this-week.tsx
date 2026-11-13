import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import { SectionHeader, Stack, Text } from "@loomarr/design-system";
import { Link } from "@tanstack/react-router";
import { ArtworkFallback } from "@/components/loomarr/artwork-fallback";
import { ChannelIdent } from "@/components/loomarr/channels/channel-ident";
import type { NewThisWeekProps } from "./new-this-week.type";

const WEEK_MS = 7 * 24 * 60 * 60 * 1000;
const HOUR_MS = 60 * 60 * 1000;
const POSTERS = 5;

// The start of "this week", quantised to the hour so the titles query key holds still between
// renders (the same reason the guide window is quantised).
const newSince = (nowMs: number) => Math.floor((nowMs - WEEK_MS) / HOUR_MS) * HOUR_MS;

// A stable number from a title's key, so a title with no channel keeps the same colour.
const seedOf = (key: string) => [...key].reduce((sum, ch) => sum + ch.charCodeAt(0), 0);

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

// "Requested by Maya · added Tuesday" (the mock). The requester is the server's name for the
// person whose request made it (#1663); a hand-made channel has none.
const addedLine = (channel: ChannelDTO, viewerName: string | undefined, timeZone?: string) => {
  const day = channel.createdAtMs
    ? new Intl.DateTimeFormat(undefined, { weekday: "long", timeZone }).format(channel.createdAtMs)
    : undefined;
  if (!channel.requestedBy) return day ? `Added ${day}` : "";
  const who = channel.requestedBy === viewerName ? "Your request" : `Requested by ${channel.requestedBy}`;
  return day ? `${who} · added ${day}` : who;
};

// The channels made this week, newest first. Each gets a row: the header counts them, so the
// list must show every one (#2763).
const freshChannels = (channels: readonly ChannelDTO[], since: number) =>
  channels
    .filter((c) => (c.createdAtMs ?? 0) >= since)
    .sort((a, b) => (b.createdAtMs ?? 0) - (a.createdAtMs ?? 0));

// NewThisWeek — Home's "New this week" (#1822 evidence): the channels that arrived this week, then
// the newest titles and the channel each landed on (#1663). Titles carry no poster yet, so every
// poster is the generated art: the landing channel's monogram when one is known, else the title's
// own letters, so none is blank (#2762).
const NewThisWeek = ({ titles, channels, since, viewerName, timeZone }: NewThisWeekProps) => {
  const fresh = freshChannels(channels, since);
  if (titles.length === 0 && fresh.length === 0) return null;
  const meta = [
    // Only five posters fit (#1822 geometry), so a longer list says how many of them are shown (#2763).
    titles.length > POSTERS
      ? `${POSTERS} of ${titles.length} titles`
      : titles.length > 0
        ? plural(titles.length, "title", "titles")
        : undefined,
    fresh.length > 0 ? plural(fresh.length, "new channel", "new channels") : undefined,
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    // `@container`: the poster grid's one-column step is measured against this section's own
    // width, not the viewport — what sits beside Home's content (the md+ rail, nothing above the
    // phone's bottom bar) is the shell's business, not this grid's.
    <Stack aria-labelledby="home-new" className="@container" gap="control" render="section">
      <SectionHeader id="home-new" title="New this week" meta={meta} />
      {/* A separate row, never a grid column beside the posters (#1822 evidence geometry table):
          sharing a track with five poster columns is what left phone poster slivers in the first
          place. It wraps its own text and action instead. */}
      {fresh.map((lead) => (
        <Link
          key={lead.id}
          to="/watch/$id"
          params={{ id: lead.id }}
          className="border-static-700 bg-static-900 text-foreground hover:border-static-500 focus-visible:ring-ring flex flex-wrap items-center gap-3 rounded-md border p-3.5 focus-visible:ring-2 focus-visible:outline-none"
        >
          <ChannelIdent name={lead.name} number={lead.number} logo={lead.logo} size={44} />
          <Stack flexBasis="0%" flexGrow={1} gap="0.5" minWidth={0}>
            <Text as="span" textRole="hint" tone="secondary">
              New channel
            </Text>
            <Text as="span" className="break-words" textRole="cardTitle" tone="primary">
              <Text as="span" textRole="mono" tone="primary">
                {lead.number}
              </Text>{" "}
              {lead.name}
            </Text>
            <Text as="span" textRole="hint" tone="secondary">
              {addedLine(lead, viewerName, timeZone)}
            </Text>
          </Stack>
          <Stack
            align="center"
            alignSelf="flex-start"
            borderColor="borderControl"
            borderRadius={8}
            borderWidth={1}
            flexShrink={0}
            height={28}
            justify="center"
            paddingHorizontal={10}
            render="span"
          >
            <Text as="span" textRole="label" tone="primary">
              Watch
            </Text>
          </Stack>
        </Link>
      ))}
      {titles.length > 0 && (
        // Two columns at phone width; one below 284px of content width — the geometry table's rule,
        // as a container query so it holds whatever chrome shares the viewport.
        <div className="grid grid-cols-2 items-start gap-3 sm:grid-cols-3 lg:grid-cols-5 @max-[284px]:grid-cols-1">
          {titles.slice(0, POSTERS).map((t) => (
            <Stack gap="1.5" key={t.key} minWidth={0}>
              <Stack
                aspectRatio={2 / 3}
                borderColor="borderDecorative"
                borderRadius={8}
                borderWidth={1}
                data-home-poster-art=""
                flexShrink={0}
                overflow="hidden"
                width="100%"
              >
                <ArtworkFallback
                  channel={
                    t.channels?.[0] ? { name: t.channels[0].name, number: t.channels[0].number } : undefined
                  }
                  label={t.name ?? t.key}
                  seed={seedOf(t.key)}
                />
              </Stack>
              <Text as="span" className="break-words" textRole="cardLabel">
                {t.name ?? t.key}
              </Text>
              {t.channels?.[0] && (
                <Text as="span" className="truncate" textRole="hint" tone="secondary">
                  {t.channels[0].name}
                </Text>
              )}
            </Stack>
          ))}
        </div>
      )}
    </Stack>
  );
};

export { addedLine, NewThisWeek, newSince };
