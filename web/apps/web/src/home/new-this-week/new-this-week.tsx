import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import { Link } from "@tanstack/react-router";
import { ArtworkFallback } from "@/components/loomarr/artwork-fallback";
import { ChannelIdent } from "@/components/loomarr/channels/channel-ident";
import { SectionHeader } from "@/components/ui/section-header";
import type { NewThisWeekProps } from "./new-this-week.type";

const WEEK_MS = 7 * 24 * 60 * 60 * 1000;
const HOUR_MS = 60 * 60 * 1000;
const POSTERS = 5;

// The start of "this week", quantised to the hour so the titles query key holds still between
// renders (the same reason the guide window is quantised).
const newSince = (nowMs: number) => Math.floor((nowMs - WEEK_MS) / HOUR_MS) * HOUR_MS;

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

// The newest channel made this week, if any, for the card that leads the row.
const newestChannel = (channels: readonly ChannelDTO[], since: number) =>
  channels
    .filter((c) => (c.createdAtMs ?? 0) >= since)
    .sort((a, b) => (b.createdAtMs ?? 0) - (a.createdAtMs ?? 0));

// NewThisWeek — Home's "New this week" (#1822 evidence): the channel that arrived this week, then
// the newest titles and the channel each landed on (#1663). Titles carry no poster yet, so every
// poster is the restrained line motif, with the landing channel's monogram when one is known
// (#1822 evidence: "a restrained line motif with a channel monogram where available").
const NewThisWeek = ({ titles, channels, since, viewerName, timeZone }: NewThisWeekProps) => {
  const fresh = newestChannel(channels, since);
  const lead = fresh[0];
  if (titles.length === 0 && !lead) return null;
  const meta = [
    titles.length > 0 ? plural(titles.length, "title", "titles") : undefined,
    fresh.length > 0 ? plural(fresh.length, "new channel", "new channels") : undefined,
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <section aria-labelledby="home-new" className="flex flex-col gap-3">
      <SectionHeader id="home-new" title="New this week" meta={meta} />
      {/* A separate row, never a grid column beside the posters (#1822 evidence geometry table):
          sharing a track with five poster columns is what left phone poster slivers in the first
          place. It wraps its own text and action instead. */}
      {lead && (
        <Link
          to="/channels/$id/watch"
          params={{ id: lead.id }}
          className="flex flex-wrap items-center gap-3 rounded-md border border-static-700 bg-static-900 p-3.5 text-foreground hover:border-signal focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <ChannelIdent name={lead.name} number={lead.number} logo={lead.logo} size={44} />
          <div className="min-w-0 flex-1">
            <span className="block text-static-400 text-xs">New channel</span>
            <span className="mt-0.5 block break-words font-semibold text-[15px]">
              <span className="font-mono text-signal">{lead.number}</span> {lead.name}
            </span>
            <span className="mt-0.5 block text-static-400 text-xs">
              {addedLine(lead, viewerName, timeZone)}
            </span>
          </div>
          <span className="inline-flex h-7 shrink-0 items-center self-start rounded-md border border-static-500 px-2.5 font-medium text-xs">
            Watch
          </span>
        </Link>
      )}
      {titles.length > 0 && (
        // Two columns (~145px) down to 390px; one below it (the geometry table: "one below 284px
        // content width" — the prototype's own breakpoint, 371px, is the viewport equivalent
        // within the 56px rail + 16px padding).
        <div className="grid grid-cols-2 items-start gap-3 max-[371px]:grid-cols-1 sm:grid-cols-3 lg:grid-cols-5">
          {titles.slice(0, POSTERS).map((t) => (
            <div key={t.key} className="flex min-w-0 flex-col gap-1.5">
              <div
                data-home-poster-art=""
                className="aspect-[2/3] w-full overflow-hidden rounded-md border border-static-700"
              >
                <ArtworkFallback
                  channel={
                    t.channels?.[0] ? { name: t.channels[0].name, number: t.channels[0].number } : undefined
                  }
                />
              </div>
              <span className="break-words font-medium text-[13px]">{t.name ?? t.key}</span>
              {t.channels?.[0] && (
                <span className="truncate text-static-400 text-xs">{t.channels[0].name}</span>
              )}
            </div>
          ))}
        </div>
      )}
    </section>
  );
};

export { addedLine, NewThisWeek, newSince };
