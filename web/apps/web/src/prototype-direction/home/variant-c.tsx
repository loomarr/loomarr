// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// C — Channel wall. No hero: Home opens on every channel as a live tile, like a TV multiview. Each
// tile is a 16:9 still with the channel number and name, what is on and a thin progress line. A
// channel still downloading its titles is a dimmer tile that says so, instead of airing an ad as
// its "now". Tonight and New this week sit below. No filled button: the tile is the control.
import { Action, SectionHeader } from "@loomarr/design-system";
import { Link } from "@tanstack/react-router";
import type { Tint } from "../shared";
import {
  AirProgress,
  NewRow,
  NumberBlock,
  OverText,
  ruleStyle,
  Segments,
  Still,
  TonightList,
  watchTo,
} from "./parts";
import { buildHome, type HomeState, type ProtoChannel } from "./stub-data";

const Tile = ({
  channel,
  tint,
  last,
  compact,
}: {
  channel: ProtoChannel;
  tint: Tint;
  last: boolean;
  compact: boolean;
}) => {
  const filling = !channel.ready;
  const name = channel.series ?? channel.title;
  return (
    <Link
      {...watchTo(channel.id)}
      style={ruleStyle(channel, tint)}
      className={`bg-static-900 focus-visible:ring-ring group relative isolate block aspect-video min-w-0 overflow-hidden rounded-lg focus-visible:ring-2 focus-visible:outline-none ${
        last ? "ring-static-0/80 ring-2" : "border-static-700 border"
      }`}
    >
      <Still hue={channel.hue} dim={filling} />
      <div
        aria-hidden
        className="from-static-950/90 via-static-950/40 absolute inset-0 bg-gradient-to-t to-transparent"
      />
      <div className={`absolute inset-x-0 top-0 flex items-center gap-2 ${compact ? "p-2" : "p-3"}`}>
        <NumberBlock channel={channel} tint={tint} />
        {!compact && <OverText textRole="label">{channel.name}</OverText>}
        {last && (
          <span className="bg-static-0 text-static-950 ml-auto rounded-full px-2 py-0.5 text-xs font-medium">
            {compact ? "Last" : "You were here"}
          </span>
        )}
      </div>
      <div
        className={`absolute inset-x-0 bottom-0 flex flex-col gap-0.5 ${compact ? "p-2 pb-2.5" : "p-3 pb-4"}`}
      >
        {filling ? (
          <>
            <OverText as="h3" textRole={compact ? "label" : "cardTitle"}>
              {compact ? channel.name : `Up next: ${channel.next.series ?? channel.next.title}`}
            </OverText>
            {!compact && (
              <OverText textRole="caption" muted>
                Ads until titles arrive
              </OverText>
            )}
            <div className="mt-1 flex items-center gap-2">
              <Segments have={channel.have} total={channel.total} className="min-w-0 flex-1" />
              <OverText textRole="caption" muted>
                {channel.have} of {channel.total}
              </OverText>
            </div>
          </>
        ) : (
          <>
            <div className="min-w-0 truncate">
              <OverText as="span" textRole={compact ? "label" : "cardTitle"}>
                {name}
              </OverText>
            </div>
            <div className="min-w-0 truncate">
              <OverText textRole="caption" muted>
                {compact
                  ? `${channel.name} · ${channel.minsLeft}m left`
                  : `${channel.episode} · ${channel.minsLeft} min left`}
              </OverText>
            </div>
          </>
        )}
      </div>
      {!filling && <AirProgress channel={channel} />}
    </Link>
  );
};

const VariantC = ({ state, device, tint }: { state: HomeState; device: string; tint: Tint }) => {
  const phone = device === "phone";
  const data = buildHome(state);
  const filling = data.channels.filter((c) => !c.ready).length;

  return (
    <div className={`flex flex-col gap-8 pb-8 ${phone ? "p-4" : "p-6"}`}>
      <section aria-labelledby="home-wall">
        <h1 className="sr-only">Home</h1>
        <SectionHeader
          id="home-wall"
          title="On now"
          meta={
            filling > 0
              ? `${filling} of ${data.channels.length} channels are still filling up`
              : `${data.channels.length} channels`
          }
        >
          <Action accessibilityRole="link" render={<Link to="/guide" />} variant="link">
            Full guide
          </Action>
        </SectionHeader>
        <div className={phone ? "grid grid-cols-2 gap-3" : "grid grid-cols-3 gap-4"}>
          {data.channels.map((c) => (
            <Tile key={c.id} channel={c} tint={tint} last={c.id === data.lastTuned?.id} compact={phone} />
          ))}
        </div>
      </section>
      <TonightList highlights={data.highlights} phone={phone} />
      <NewRow titles={data.titles} newChannel={data.newChannel} phone={phone} tint={tint} />
    </div>
  );
};

export { VariantC };
