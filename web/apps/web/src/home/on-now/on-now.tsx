import type { GuideChannelState } from "@loomarr/core/guide";
import { guideChannelState } from "@loomarr/core/guide";
import { surfChannelData } from "@loomarr/ui";
import { Link } from "@tanstack/react-router";
import { ArtworkFallback } from "@/components/loomarr/artwork-fallback";
import { ChannelIdent } from "@/components/loomarr/channels/channel-ident";
import { Image } from "@/components/ui/image";
import { ListGroup, ListRow } from "@/components/ui/list-row";
import { SectionHeader, SectionHeaderAction } from "@/components/ui/section-header";
import type { StatusTone } from "@/components/ui/status-dot";
import type { OnNowProps } from "./on-now.type";

const MAX_CHANNELS = 6;

// The actual, truthful lifecycle sentence for a channel with no current block — never a
// first-channel prompt, since the channel already exists (#1822 evidence).
const lifecycle = (state: GuideChannelState): { label: string; tone: StatusTone } => {
  if (state.health === "paused") return { label: "Paused", tone: "off" };
  if (state.health === "creating") return { label: "Building schedule", tone: "progress" };
  if (state.health === "error") return { label: "Couldn’t reach this channel", tone: "error" };
  if (state.health === "drift") return { label: "Catching up", tone: "warn" };
  return { label: "No programme scheduled now", tone: "off" };
};

// OnNow — Home's "On now" (#1822 evidence): up to six currently scheduled live channels in
// channel-number order, from the same guide facts the Guide reads. A household with channels but
// nothing playing right now gets "Your channels" instead: actual lifecycle or schedule-gap
// wording and View channel, never a first-channel prompt — the channels already exist.
const OnNow = ({ layout, nowMs }: OnNowProps) => {
  const ordered = [...layout.channels].sort((a, b) => a.source.number - b.source.number);
  const onNow = ordered
    .map((channel) => ({
      channel,
      data: surfChannelData(channel, nowMs, layout.timezone),
      airing: channel.airings.find((a) => a.source.startMs <= nowMs && a.source.stopMs > nowMs)?.source,
    }))
    .filter(
      (
        c,
      ): c is typeof c & {
        data: { now: NonNullable<(typeof c)["data"]["now"]> };
        airing: NonNullable<(typeof c)["airing"]>;
      } => Boolean(c.data.now && c.airing),
    )
    .slice(0, MAX_CHANNELS);

  if (onNow.length === 0) {
    return (
      <section aria-labelledby="home-on-now">
        <SectionHeader id="home-on-now" title="Your channels" />
        <ListGroup aria-labelledby="home-on-now">
          {ordered.map(({ source }) => {
            const { label, tone } = lifecycle(guideChannelState(source));
            return (
              <ListRow
                key={source.channelId}
                tone={tone}
                title={
                  <span className="flex items-center gap-2">
                    <span className="font-mono text-signal">{String(source.number).padStart(2, "0")}</span>
                    {source.name}
                  </span>
                }
                sub={label}
                action={
                  <Link
                    to="/channels/$id"
                    params={{ id: source.channelId }}
                    className="text-[13px] text-static-400 hover:text-foreground"
                  >
                    View channel <span aria-hidden="true">→</span>
                  </Link>
                }
              />
            );
          })}
        </ListGroup>
      </section>
    );
  }

  return (
    <section aria-labelledby="home-on-now">
      <SectionHeader id="home-on-now" title="On now">
        <SectionHeaderAction render={<Link to="/guide" />}>Full guide</SectionHeaderAction>
      </SectionHeader>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {onNow.map(({ channel, data, airing }) => (
          <Link
            key={channel.source.channelId}
            to="/channels/$id/watch"
            params={{ id: channel.source.channelId }}
            aria-label={`Watch ${data.channelName} live`}
            className="block min-w-0 overflow-hidden rounded-lg border border-static-700 bg-static-900 hover:border-static-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <div className="relative aspect-video w-full overflow-hidden bg-static-800">
              {airing.thumbImage ? (
                <Image
                  image={airing.thumbImage}
                  alt=""
                  sizes="(min-width: 1024px) 33vw, (min-width: 640px) 50vw, 100vw"
                  fallback={
                    <ArtworkFallback channel={{ name: data.channelName, number: channel.source.number }} />
                  }
                  className="size-full object-cover"
                />
              ) : airing.thumbUrl ? (
                <img src={airing.thumbUrl} alt="" className="size-full object-cover" />
              ) : (
                <ArtworkFallback channel={{ name: data.channelName, number: channel.source.number }} />
              )}
            </div>
            <div className="flex flex-col gap-1.5 p-3.5">
              <div className="flex items-center gap-2">
                <ChannelIdent
                  name={data.channelName}
                  number={channel.source.number}
                  logo={data.channelLogoUri}
                  size={28}
                />
                <span className="font-mono text-signal text-sm">
                  {String(channel.source.number).padStart(2, "0")}
                </span>
                <span className="truncate text-static-400 text-xs">{data.channelName}</span>
              </div>
              <h3 className="m-0 break-words font-semibold text-sm">
                {data.now.seriesTitle ?? data.now.title}
              </h3>
              {data.now.episodeLabel && (
                <p className="m-0 text-static-400 text-xs">{data.now.episodeLabel}</p>
              )}
              <p className="m-0 font-mono text-static-400 text-xs">{data.now.timeLabel}</p>
              <span className="font-medium text-signal text-sm">
                Watch live <span aria-hidden="true">→</span>
              </span>
            </div>
          </Link>
        ))}
      </div>
    </section>
  );
};

export { OnNow };
