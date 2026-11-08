// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// A — Today. Home as it ships: "N channels", Recently tuned, On now, Tonight, New this week and
// Channel ideas (member view). HomeStrip, Tonight, ChannelIdent and ArtworkFallback are the real
// components; Recently tuned, On now, New this week and Channel ideas take their data from live
// queries, so they are faithful copies fed stub data. The copies use the frame's width, not the
// browser's, for their breakpoints. Watching now and Your requests are left out.
import { Action, Badge, SectionHeader, StatusDot } from "@loomarr/design-system";
import { Link } from "@tanstack/react-router";
import { ArtworkFallback } from "@/components/loomarr/artwork-fallback";
import { ChannelIdent } from "@/components/loomarr/channels/channel-ident";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { Button } from "@/components/ui/button";
import { HomeStrip } from "@/home/home-strip";
import { Tonight } from "@/home/tonight";
import type { Tint } from "../shared";
import { Still, TonightList, watchTo } from "./parts";
import { buildHome, type HomeState, type ProtoChannel, pad, TIME_ZONE } from "./stub-data";

// While titles download a channel's "now" is a commercial break: today's cards say so, with the
// spot's name, and have no artwork to draw.
const nowOf = (c: ProtoChannel) =>
  c.ready
    ? { title: c.series ?? c.title, episode: c.episode, range: c.timeRange, art: true }
    : { title: `Commercials · ${c.adSpot}`, episode: undefined, range: c.adRange, art: false };

const Art = ({ channel, art }: { channel: ProtoChannel; art: boolean }) =>
  art ? (
    <div className="relative size-full">
      <Still hue={channel.hue} />
    </div>
  ) : (
    <ArtworkFallback channel={{ name: channel.name, number: channel.number }} />
  );

const RecentlyTuned = ({ channel, phone }: { channel: ProtoChannel; phone: boolean }) => {
  const now = nowOf(channel);
  return (
    <section aria-labelledby="home-recently-tuned">
      <SectionHeader id="home-recently-tuned" title="Recently tuned" />
      <div
        className={`border-static-700 bg-static-900 flex gap-4 rounded-lg border p-4 ${phone ? "flex-col items-start" : "items-center"}`}
      >
        <div className={`aspect-video shrink-0 overflow-hidden rounded-md ${phone ? "w-full" : "w-48"}`}>
          <Art channel={channel} art={now.art} />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <ChannelIdent name={channel.name} number={channel.number} />
            <span className="text-foreground font-mono text-sm">{pad(channel.number)}</span>
            <span className="text-static-400 text-sm">{channel.name}</span>
          </div>
          <h3 className="mt-2.5 text-base font-semibold break-words">{now.title}</h3>
          <p className="text-static-400 m-0 text-sm">Join what’s on now.</p>
        </div>
        <Button render={<Link {...watchTo(channel.id)} />}>Watch live</Button>
      </div>
    </section>
  );
};

const OnNow = ({ channels, phone }: { channels: ProtoChannel[]; phone: boolean }) => (
  <section aria-labelledby="home-on-now">
    <SectionHeader id="home-on-now" title="On now">
      <Action accessibilityRole="link" render={<Link to="/guide" />} variant="link">
        Full guide
      </Action>
    </SectionHeader>
    <div className={`grid gap-3 ${phone ? "grid-cols-1" : "grid-cols-3"}`}>
      {channels.map((c) => {
        const now = nowOf(c);
        return (
          <Link
            key={c.id}
            {...watchTo(c.id)}
            className="border-static-700 bg-static-900 hover:border-static-500 focus-visible:ring-ring block min-w-0 overflow-hidden rounded-lg border focus-visible:ring-2 focus-visible:outline-none"
          >
            <div className="bg-static-800 relative aspect-video w-full overflow-hidden">
              <Art channel={c} art={now.art} />
            </div>
            <div className="flex flex-col gap-1.5 p-3.5">
              <div className="flex items-center gap-2">
                <ChannelIdent name={c.name} number={c.number} size={28} />
                <span className="text-foreground font-mono text-sm">{pad(c.number)}</span>
                <span className="text-static-400 truncate text-xs">{c.name}</span>
              </div>
              <h3 className="m-0 text-base font-semibold break-words">{now.title}</h3>
              {now.episode && <p className="text-static-400 m-0 text-xs">{now.episode.replace(" ", "")}</p>}
              <p className="text-static-400 m-0 font-mono text-xs">{now.range}</p>
              <span className="text-foreground text-sm font-medium">
                Watch live <span aria-hidden="true">→</span>
              </span>
            </div>
          </Link>
        );
      })}
    </div>
  </section>
);

const NewThisWeek = ({ data, phone }: { data: ReturnType<typeof buildHome>; phone: boolean }) => (
  <section aria-labelledby="home-new" className="flex flex-col gap-3">
    <SectionHeader
      id="home-new"
      title="New this week"
      meta={`${data.titles.length} titles · 1 new channel`}
    />
    <Link
      to="/guide"
      className="border-static-700 bg-static-900 text-foreground hover:border-static-500 flex flex-wrap items-center gap-3 rounded-md border p-3.5"
    >
      <ChannelIdent name={data.newChannel.name} number={data.newChannel.number} size={44} />
      <div className="min-w-0 flex-1">
        <span className="text-static-400 block text-xs">New channel</span>
        <span className="mt-0.5 block text-[15px] font-semibold break-words">
          <span className="font-mono">{data.newChannel.number}</span> {data.newChannel.name}
        </span>
        <span className="text-static-400 mt-0.5 block text-xs">{data.newChannel.line}</span>
      </div>
      <span className="border-static-500 inline-flex h-7 shrink-0 items-center self-start rounded-md border px-2.5 text-xs font-medium">
        Watch
      </span>
    </Link>
    <div className={`grid items-start gap-3 ${phone ? "grid-cols-2" : "grid-cols-5"}`}>
      {data.titles.slice(0, phone ? 4 : 5).map((t) => (
        <div key={t.key} className="flex min-w-0 flex-col gap-1.5">
          <div className="border-static-700 aspect-[2/3] w-full overflow-hidden rounded-md border">
            <ArtworkFallback channel={{ name: t.channelName, number: t.channelNumber }} />
          </div>
          <span className="text-sm font-medium break-words">{t.name}</span>
          <span className="text-static-400 truncate text-xs">{t.channelName}</span>
        </div>
      ))}
    </div>
  </section>
);

const IDEAS = [
  {
    name: "Rainy Day Cartoons",
    pitch: "Gentle animated shorts for slow afternoons.",
    count: "14 titles",
    avail: "All in your library",
    reason: "Because you watch Saturday Toons",
  },
  {
    name: "Weekend Road Trips",
    pitch: "Travel documentaries, one destination at a time.",
    count: "9 titles",
    avail: "3 still to download",
    reason: "Nothing on a channel yet",
  },
  {
    name: "Cosy Mysteries",
    pitch: "Small-town puzzles with a pot of tea.",
    count: "11 titles",
    avail: "All in your library",
    reason: "Because you watch Coast 90s",
  },
];

const ChannelIdeas = ({ phone }: { phone: boolean }) => (
  <section aria-labelledby="home-ideas">
    <SectionHeader id="home-ideas" title="Channel ideas" meta="Made from what’s in your library" />
    <div className={`grid gap-3 ${phone ? "grid-cols-1" : "grid-cols-3"}`}>
      {IDEAS.map((idea, i) => (
        <article
          key={idea.name}
          className="border-static-700 bg-static-900 flex min-w-0 flex-col overflow-hidden rounded-lg border"
        >
          <div aria-hidden className="grid grid-cols-4 gap-1.5 px-3 pt-3">
            {[0, 1, 2, 3].map((k) => (
              <div key={k} className="border-static-700 bg-static-800 aspect-[2/3] rounded-[6px] border" />
            ))}
          </div>
          <div className="flex flex-col gap-1 px-4 pt-3.5">
            <span className="text-static-400 text-xs">{idea.reason}</span>
            <h3 className="m-0 text-base leading-[1.3] font-semibold">{idea.name}</h3>
            <p className="text-static-400 m-0 text-sm text-pretty">{idea.pitch}</p>
          </div>
          <div className="flex items-center gap-2 px-4 pt-2.5 text-xs">
            <StatusDot tone={i === 1 ? "progress" : "ok"} label="" size="compact" />
            <span>
              {idea.count}
              <span className="text-static-400"> · {idea.avail}</span>
            </span>
          </div>
          <div className="mt-auto flex items-center gap-2 p-4">
            {i === 2 ? (
              <Badge shape="square" tone="suggest">
                Waiting for an admin
              </Badge>
            ) : (
              <>
                <Button variant="outline" size="sm">
                  Request
                </Button>
                <Button variant="ghost" size="sm">
                  Hide
                </Button>
              </>
            )}
          </div>
        </article>
      ))}
    </div>
  </section>
);

const VariantA = ({ state, device }: { state: HomeState; device: string; tint: Tint }) => {
  const phone = device === "phone";
  const data = buildHome(state);
  return (
    <div className="flex flex-col">
      <PageHeader title="Home" />
      <div className={`flex flex-col ${phone ? "gap-6 p-4" : "max-w-overview mx-auto w-full gap-8 p-6"}`}>
        <HomeStrip state="ok" count={data.channels.length} isAdmin={false} onRetry={() => undefined} />
        {data.lastTuned && <RecentlyTuned channel={data.lastTuned} phone={phone} />}
        <OnNow channels={data.channels} phone={phone} />
        {phone ? (
          <TonightList highlights={data.highlights} phone />
        ) : (
          <Tonight highlights={data.highlights} timeZone={TIME_ZONE} />
        )}
        <NewThisWeek data={data} phone={phone} />
        <ChannelIdeas phone={phone} />
      </div>
    </div>
  );
};

export { VariantA };
