// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// Pieces variants B and C share: the channel number block, progress lines, the "still filling up"
// pieces, Tonight and New this week. Variant A keeps its own faithful copies of today's sections.
import { formatGuideTime } from "@loomarr/core/guide";
import { Action, SectionHeader, Text } from "@loomarr/design-system";
import { Link } from "@tanstack/react-router";
import { highlightReason } from "@/home/tonight";
import { eraTint, standInStill, type Tint } from "../shared";
import { type HomeData, type ProtoChannel, type ProtoTitle, pad, TIME_ZONE } from "./stub-data";

// One channel's colour, only when the tint toggle is on; neutral otherwise.
const tintOf = (channel: Pick<ProtoChannel, "era">, tint: Tint) => eraTint(channel.era, tint);

const NumberBlock = ({ channel, tint }: { channel: ProtoChannel; tint: Tint }) => {
  const color = tintOf(channel, tint);
  return (
    <span
      style={color ? { backgroundColor: color } : undefined}
      className={`inline-flex h-6 min-w-8 shrink-0 items-center justify-center rounded-sm px-1.5 font-mono text-sm font-semibold ${
        color ? "text-static-950" : "bg-static-950/75 text-static-0 ring-static-0/20 ring-1"
      }`}
    >
      {pad(channel.number)}
    </span>
  );
};

// The 2px card rule on top of a card, in the channel's colour when tinted.
const ruleStyle = (channel: ProtoChannel, tint: Tint) => {
  const color = tintOf(channel, tint);
  return color ? { borderTop: `2px solid ${color}` } : undefined;
};

// How far through its programme a channel is: a thin line along the bottom of a picture.
const AirProgress = ({ channel }: { channel: ProtoChannel }) => (
  <div aria-hidden className="bg-static-0/20 absolute inset-x-0 bottom-0 h-[3px]">
    <div
      className="bg-static-0 h-full"
      style={{
        width: `${Math.round(((channel.runtimeMins - channel.minsLeft) / channel.runtimeMins) * 100)}%`,
      }}
    />
  </div>
);

// Titles downloaded so far, one tick per title. Neutral: it is not a signal, just a count.
const Segments = ({ have, total, className = "" }: { have: number; total: number; className?: string }) => (
  <div aria-hidden className={`flex gap-0.5 ${className}`}>
    {Array.from({ length: total }, (_, i) => `tick-${i}`).map((id, i) => (
      <span key={id} className={`h-1 flex-1 rounded-full ${i < have ? "bg-static-100" : "bg-static-0/20"}`} />
    ))}
  </div>
);

const Still = ({ hue, title, dim }: { hue: number; title?: string; dim?: boolean }) => (
  <img
    src={standInStill(hue, title)}
    alt=""
    className={`absolute inset-0 size-full object-cover ${dim ? "opacity-70 saturate-50" : ""}`}
  />
);

const watchTo = (id: string) => ({ to: "/watch/$id" as const, params: { id } });

// What a channel with a programme airing is showing, in the words a person would use.
const nowLine = (c: ProtoChannel) =>
  c.series ? `${c.episode} “${c.title}” · ${c.minsLeft} min left` : `${c.episode} · ${c.minsLeft} min left`;

// "Ads until titles arrive · 3 of 12 downloaded": what a channel without its shows says.
const fillingLine = (c: ProtoChannel) => `Ads until titles arrive · ${c.have} of ${c.total} downloaded`;

// ---------------------------------------------------------------------------------------------
// Rows for channels still downloading their titles: the better state (no wall of ads).

const FillingUp = ({
  channels,
  tint,
  phone,
  title = "Still filling up",
  note,
}: {
  channels: ProtoChannel[];
  tint: Tint;
  phone: boolean;
  title?: string;
  note?: string;
}) => {
  if (channels.length === 0) return null;
  return (
    <section aria-labelledby="home-filling">
      <SectionHeader id="home-filling" title={title} meta={note} />
      <ul className="border-static-700 bg-static-900 m-0 flex list-none flex-col overflow-hidden rounded-lg border p-0">
        {channels.map((c, i) => (
          <li key={c.id} className={i > 0 ? "border-static-700 border-t" : undefined}>
            <Link
              {...watchTo(c.id)}
              className="hover:bg-static-800 focus-visible:ring-ring flex items-center gap-3 px-4 py-3 focus-visible:ring-2 focus-visible:outline-none focus-visible:ring-inset"
            >
              <NumberBlock channel={c} tint={tint} />
              <div className="min-w-0 flex-1">
                <p className="m-0 truncate text-sm font-semibold">
                  {c.name}
                  <span className="text-static-400 font-normal">
                    {" "}
                    · first up: {c.next.series ?? c.next.title}
                  </span>
                </p>
                <p className="text-static-400 m-0 text-xs">
                  {phone
                    ? `${c.have} of ${c.total} downloaded · ads until then`
                    : "Plays ads until the first titles arrive"}
                </p>
              </div>
              {!phone && (
                <div className="flex w-48 shrink-0 flex-col gap-1.5">
                  <Segments have={c.have} total={c.total} />
                  <span className="text-static-400 text-xs">
                    {c.have} of {c.total} downloaded
                  </span>
                </div>
              )}
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
};

// ---------------------------------------------------------------------------------------------
// Tonight: today's highlights, laid out for the frame's own width (the real row reads the
// browser's width, which would lie inside a phone frame on a desktop screen).

const TonightList = ({ highlights, phone }: { highlights: HomeData["highlights"]; phone: boolean }) => {
  if (highlights.length === 0) return null;
  return (
    <section aria-labelledby="home-tonight">
      <SectionHeader id="home-tonight" title="Tonight">
        <Action accessibilityRole="link" render={<Link to="/guide" />} variant="link">
          Full guide
        </Action>
      </SectionHeader>
      <ul className="border-static-700 bg-static-900 m-0 flex list-none flex-col overflow-hidden rounded-lg border p-0">
        {highlights.map((h, i) => (
          <li key={h.airing.scheduleBlockId} className={i > 0 ? "border-static-700 border-t" : undefined}>
            <Link
              {...watchTo(h.channelId)}
              className={`hover:bg-static-800 grid items-center gap-x-3 gap-y-0.5 px-4 py-3 ${
                phone ? "grid-cols-[80px_36px_minmax(0,1fr)]" : "grid-cols-[80px_36px_minmax(0,1fr)_auto]"
              }`}
            >
              <span className="font-mono text-sm">{formatGuideTime(h.airing.startMs, TIME_ZONE)}</span>
              <span className="font-mono text-sm font-semibold">{pad(h.channelNumber)}</span>
              <span className="truncate text-sm font-semibold">
                {h.airing.series ?? h.airing.title}
                {h.airing.series && <span className="text-static-400 font-normal"> · {h.airing.title}</span>}
              </span>
              <span className={`text-static-400 text-xs ${phone ? "col-start-3" : ""}`}>
                {highlightReason(h, TIME_ZONE)}
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
};

// ---------------------------------------------------------------------------------------------
// New this week: the newest titles as small pictures (not hatched poster tiles), and the channel
// that arrived this week.

const NewRow = ({
  titles,
  newChannel,
  phone,
  tint,
}: {
  titles: ProtoTitle[];
  newChannel: HomeData["newChannel"];
  phone: boolean;
  tint: Tint;
}) => (
  <section aria-labelledby="home-new">
    <SectionHeader id="home-new" title="New this week" meta={`${titles.length} titles · 1 new channel`} />
    <div className="flex flex-col gap-3">
      <Link
        to="/guide"
        className="border-static-700 bg-static-900 hover:border-static-500 flex items-center gap-3 rounded-lg border p-3.5"
      >
        <span
          className={`inline-flex h-8 min-w-9 items-center justify-center rounded-sm font-mono text-sm font-semibold ${
            tint === "era" ? "bg-signal text-static-950" : "bg-static-800 text-foreground"
          }`}
        >
          {pad(newChannel.number)}
        </span>
        <div className="min-w-0 flex-1">
          <p className="text-static-400 m-0 text-xs">New channel</p>
          <p className="m-0 truncate text-sm font-semibold">{newChannel.name}</p>
          <p className="text-static-400 m-0 truncate text-xs">{newChannel.line}</p>
        </div>
      </Link>
      <div className={phone ? "-mx-4 flex snap-x gap-3 overflow-x-auto px-4 pb-1" : "grid grid-cols-5 gap-4"}>
        {titles.map((t) => (
          <div
            key={t.key}
            className={`flex min-w-0 flex-col gap-1.5 ${phone ? "w-40 shrink-0 snap-start" : ""}`}
          >
            <div className="border-static-700 relative aspect-video w-full overflow-hidden rounded-md border">
              <Still hue={t.hue} />
            </div>
            <span className="truncate text-sm font-semibold">{t.name}</span>
            <span className="text-static-400 -mt-1 truncate text-xs">On {t.channelName}</span>
          </div>
        ))}
      </div>
    </div>
  </section>
);

// ---------------------------------------------------------------------------------------------

// Hero-style white text on a picture: DS roles, in the page's brightest neutral.
const OverText = ({
  textRole,
  as = "span",
  children,
  muted,
}: {
  textRole: "display" | "title" | "cardTitle" | "body" | "caption" | "label";
  as?: "span" | "h2" | "h3" | "p";
  children: React.ReactNode;
  muted?: boolean;
}) => (
  <Text
    as={as}
    textRole={textRole}
    style={{ color: muted ? "var(--color-static-100)" : "var(--color-static-0)", margin: 0 }}
  >
    {children}
  </Text>
);

export {
  AirProgress,
  FillingUp,
  fillingLine,
  NewRow,
  NumberBlock,
  nowLine,
  OverText,
  ruleStyle,
  Segments,
  Still,
  TonightList,
  tintOf,
  watchTo,
};
