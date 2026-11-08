// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// B — Hero. The review's proposal: Home opens on the channel you watched last, full width, a live
// still under a bottom scrim with the channel chip and title, Watch live as the only button; then
// On now, Tonight and New this week. Channel ideas has moved to Requests.
import { Action, SectionHeader } from "@loomarr/design-system";
import { Link } from "@tanstack/react-router";
import { LivePicture, type Tint } from "../shared";
import {
  AirProgress,
  FillingUp,
  NewRow,
  NumberBlock,
  nowLine,
  OverText,
  ruleStyle,
  Segments,
  Still,
  TonightList,
  watchTo,
} from "./parts";
import { buildHome, type HomeState, type ProtoChannel } from "./stub-data";

const Hero = ({
  channel,
  phone,
  tint,
  firstRun,
}: {
  channel: ProtoChannel;
  phone: boolean;
  tint: Tint;
  firstRun: boolean;
}) => {
  const filling = !channel.ready;
  // The next programme, which is what a channel still downloading is waiting to play.
  const title = filling ? (channel.next.series ?? channel.next.title) : (channel.series ?? channel.title);
  const line = filling
    ? `Up next at ${channel.next.at} · ${channel.next.series ? `“${channel.next.title}”` : ""}`.replace(
        / · $/,
        "",
      )
    : nowLine(channel);
  return (
    <section
      aria-labelledby="home-hero"
      className={`relative isolate overflow-hidden ${phone ? "aspect-video" : "h-[440px]"}`}
    >
      {channel.id === "ch-7" ? <LivePicture /> : <Still hue={channel.hue} />}
      <div
        aria-hidden
        className="from-static-950 via-static-950/70 absolute inset-x-0 bottom-0 h-[78%] bg-gradient-to-t to-transparent"
      />
      <div
        className={`absolute inset-x-0 bottom-0 flex flex-col ${phone ? "gap-1.5 p-4 pb-5" : "max-w-[720px] gap-2 p-8 pb-9"}`}
      >
        <div className="flex items-center gap-2">
          <NumberBlock channel={channel} tint={tint} />
          <OverText textRole="label">{channel.name}</OverText>
          <OverText textRole="caption" muted>
            · {firstRun ? "Start here" : "You were watching"}
          </OverText>
        </div>
        <OverText as="h2" textRole={phone ? "title" : "display"}>
          <span id="home-hero">{title}</span>
        </OverText>
        <OverText textRole="body" muted>
          {line}
        </OverText>
        {filling && (
          <div className="flex items-center gap-3">
            {!phone && <Segments have={channel.have} total={channel.total} className="w-28 shrink-0" />}
            <OverText textRole="caption" muted>
              Ads until titles arrive · {channel.have} of {channel.total} downloaded
            </OverText>
          </div>
        )}
        <div className={phone ? "mt-1.5" : "mt-3"}>
          <Action
            accessibilityRole="link"
            density={phone ? "touch" : "pointer"}
            render={<Link {...watchTo(channel.id)} />}
          >
            Watch live
          </Action>
        </div>
      </div>
      {!filling && <AirProgress channel={channel} />}
    </section>
  );
};

// A compact On now card: a still with its channel number, then what is on and the time left.
const OnNowCard = ({ channel, tint, phone }: { channel: ProtoChannel; tint: Tint; phone: boolean }) => (
  <Link
    {...watchTo(channel.id)}
    style={ruleStyle(channel, tint)}
    className={`border-static-700 bg-static-900 hover:border-static-500 focus-visible:ring-ring block min-w-0 overflow-hidden rounded-lg border focus-visible:ring-2 focus-visible:outline-none ${
      phone ? "w-60 shrink-0 snap-start" : "w-[220px] shrink-0"
    }`}
  >
    <div className="bg-static-800 relative aspect-video w-full overflow-hidden">
      <Still hue={channel.hue} />
      <span className="absolute top-2 left-2">
        <NumberBlock channel={channel} tint={tint} />
      </span>
      <AirProgress channel={channel} />
    </div>
    <div className="flex flex-col gap-0.5 p-3">
      <h3 className="m-0 truncate text-[15px] font-semibold">{channel.series ?? channel.title}</h3>
      <p className="text-static-400 m-0 truncate text-xs">
        {channel.name} · {channel.minsLeft} min left
      </p>
    </div>
  </Link>
);

const VariantB = ({ state, device, tint }: { state: HomeState; device: string; tint: Tint }) => {
  const phone = device === "phone";
  const data = buildHome(state);
  const hero = data.lastTuned ?? data.lowest;
  const others = data.channels.filter((c) => c.id !== hero.id);
  const ready = others.filter((c) => c.ready);
  const filling = others.filter((c) => !c.ready);
  const pad4 = phone ? "px-4" : "px-6";

  return (
    <div className="flex flex-col gap-8 pb-8">
      <h1 className="sr-only">Home</h1>
      <Hero channel={hero} phone={phone} tint={tint} firstRun={!data.lastTuned} />
      <div className={`flex flex-col gap-8 ${pad4}`}>
        {ready.length > 0 && (
          <section aria-labelledby="home-on-now">
            <SectionHeader id="home-on-now" title="On now">
              <Action accessibilityRole="link" render={<Link to="/guide" />} variant="link">
                Full guide
              </Action>
            </SectionHeader>
            <div className={`flex gap-4 ${phone ? "-mx-4 snap-x overflow-x-auto px-4 pb-1" : "flex-wrap"}`}>
              {ready.map((c) => (
                <OnNowCard key={c.id} channel={c} tint={tint} phone={phone} />
              ))}
            </div>
          </section>
        )}
        <FillingUp
          channels={filling}
          tint={tint}
          phone={phone}
          note={`${filling.length} channels are still downloading their titles`}
        />
        <TonightList highlights={data.highlights} phone={phone} />
        <NewRow titles={data.titles} newChannel={data.newChannel} phone={phone} tint={tint} />
      </div>
    </div>
  );
};

export { VariantB };
