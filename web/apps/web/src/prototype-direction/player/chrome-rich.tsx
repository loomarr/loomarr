// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// Variant D: Rich flat. Variant C's structure (four controls, the rest behind More) with the finish
// of a shipped player. The richness is content and type, never effects: a channel chip, a large
// title, a year · rating · runtime · genre line, a one-line synopsis, a scrubber that shows the
// evening's programmes and breaks with times at both ends, an Up next card, and labelled keys on
// desktop. Phones keep the title block, the scrubber and the four controls, and nothing else.
import { Text } from "@loomarr/design-system";
import type { ReactNode } from "react";
import type { ChromeProps } from "./chrome-classic";
import { b1, channel, formatTime, lastChannel, MINUTE, NOW, nowMeta, segments } from "./data";
import { type Density, flat, GoLive, LastChannel, PlayDisc, scrimStyle, Tool } from "./flat";
import { useWidth } from "./playbar";
import type { Scenario } from "./scenario";
import { lagLabel, stationLine } from "./scenario";
import { ChannelList, Cover, MoreBody, Sheet } from "./sheets";

const programmes = segments.filter((s) => s.kind === "programme");
const current = programmes.find((s) => s.id === "p2");
const next = programmes.find((s) => s.id === "p3");
const lead = segments.find((s) => s.id === "b1");
const tail = segments.find((s) => s.id === "b2");
if (!(current && next && lead && tail)) throw new Error("stub data: p2, p3, b1 and b2 exist");

const Dot = ({ density }: { density: Density }) => (
  <Text density={density} textRole="body" tone="muted">
    ·
  </Text>
);

const RatingBadge = ({ density, rating }: { density: Density; rating: string }) => (
  <span style={{ border: `1px solid ${flat.textMuted}`, borderRadius: 4, padding: "0 6px" }}>
    <Text density={density} strong textRole="cardMeta" tone="secondary">
      {rating}
    </Text>
  </span>
);

// The same blue as the break segments on the scrubber, so the badge and the track agree.
const BreakBadge = ({ density }: { density: Density }) => (
  <span
    style={{
      background: flat.infoSurface,
      border: `1px solid ${flat.info}`,
      borderRadius: 4,
      padding: "0 6px",
    }}
  >
    <Text density={density} strong textRole="cardMeta" tone="primary">
      Ad break
    </Text>
  </span>
);

const MetaRow = ({ density, items }: { density: Density; items: ReactNode[] }) => (
  <div style={{ alignItems: "center", display: "flex", flexWrap: "wrap", gap: "2px 10px" }}>
    {items.map((item, i) => (
      // biome-ignore lint/suspicious/noArrayIndexKey: a fixed, ordered list
      <span key={i} style={{ alignItems: "center", display: "inline-flex", gap: 10 }}>
        {i > 0 ? <Dot density={density} /> : null}
        {typeof item === "string" ? (
          <Text density={density} textRole="body" tone="secondary">
            {item}
          </Text>
        ) : (
          item
        )}
      </span>
    ))}
  </div>
);

// The channel: a number block (the only thing the era tint colours) and its name.
const ChannelChip = ({ density, tint }: { density: Density; tint?: string }) => (
  <div style={{ alignItems: "center", display: "flex", gap: 10 }}>
    <span
      style={{
        alignItems: "baseline",
        background: tint ?? flat.amber,
        borderRadius: 6,
        display: "inline-flex",
        gap: 6,
        padding: "3px 10px",
      }}
    >
      <Text density={density} textRole="cardChannelNumber" tone="inverse">
        CH
      </Text>
      <Text density={density} textRole="guideNumber" tone="inverse">
        {channel.number}
      </Text>
    </span>
    <Text density={density} strong textRole="label" tone="primary">
      {channel.name}
    </Text>
  </div>
);

// The stretch around now: the break before this programme, the programme, the break after it. Drawn
// time-true so a programme looks as long as it is, with a break never thinner than a legible tick.
const SPAN = { endMs: tail.stopMs, startMs: lead.startMs };
const timeline = (width: number) => {
  const total = SPAN.endMs - SPAN.startMs;
  const xAt = (ms: number) => (Math.min(Math.max(ms - SPAN.startMs, 0), total) / total) * width;
  const boxes = [lead, current, tail].map((segment) => {
    const x = xAt(segment.startMs);
    return { segment, width: Math.max(10, xAt(segment.stopMs) - x - 3), x };
  });
  return { boxes, xAt };
};

// The evening as one track: programmes in white, commercial breaks in blue, played part solid.
const Scrubber = ({
  density,
  pointer,
  scenario,
}: {
  density: Density;
  pointer: boolean;
  scenario: Scenario;
}) => {
  const { ref, width } = useWidth();
  const { boxes, xAt } = timeline(width);
  const head = xAt(Math.min(scenario.playheadMs, NOW));
  const liveX = xAt(NOW);
  const behind = scenario.lagMs > 1000;
  const track = pointer ? 6 : 5;
  const holder = pointer ? 20 : 16;
  const thumb = pointer ? 14 : 12;
  const edge = (ms: number) => (
    <div style={{ alignItems: "center", display: "flex", height: holder }}>
      <Text density={density} textRole="compact" tone="secondary">
        {formatTime(ms)}
      </Text>
    </div>
  );
  return (
    <div style={{ alignItems: "flex-start", display: "flex", gap: 14 }}>
      {pointer ? edge(SPAN.startMs) : null}
      <div style={{ flex: 1, minWidth: 0 }}>
        <div ref={ref} style={{ height: holder, position: "relative" }}>
          {width > 0 ? (
            <>
              {boxes.map((box) => {
                const isBreak = box.segment.kind === "break";
                const played = Math.min(Math.max(head - box.x, 0), box.width);
                return (
                  <div
                    key={box.segment.id}
                    style={{
                      background: isBreak ? flat.infoSurface : "rgba(255, 255, 255, 0.2)",
                      borderRadius: track / 2,
                      height: track,
                      left: box.x,
                      overflow: "hidden",
                      position: "absolute",
                      top: (holder - track) / 2,
                      width: box.width,
                    }}
                  >
                    <div
                      style={{
                        background: isBreak ? flat.info : flat.text,
                        height: "100%",
                        width: played,
                      }}
                    />
                    {isBreak && played === 0 ? (
                      <div style={{ background: flat.info, height: "100%", opacity: 0.55, width: "100%" }} />
                    ) : null}
                  </div>
                );
              })}
              {behind ? (
                <div
                  style={{
                    background: flat.live,
                    height: thumb + 4,
                    left: liveX - 1,
                    position: "absolute",
                    top: (holder - thumb - 4) / 2,
                    width: 2,
                  }}
                />
              ) : null}
              <div
                style={{
                  background: flat.text,
                  borderRadius: thumb,
                  height: thumb,
                  left: head - thumb / 2,
                  position: "absolute",
                  top: (holder - thumb) / 2,
                  width: thumb,
                }}
              />
            </>
          ) : null}
        </div>
        {pointer ? (
          <div aria-hidden style={{ height: 18, position: "relative" }}>
            {width > 0
              ? boxes
                  .filter((b) => b.segment.kind === "break")
                  .map((b) => (
                    <span
                      key={b.segment.id}
                      style={{
                        left: b.x + b.width / 2,
                        position: "absolute",
                        top: 4,
                        transform: "translateX(-50%)",
                      }}
                    >
                      <Text density={density} textRole="guideMeta" tone="muted">
                        Break
                      </Text>
                    </span>
                  ))
              : null}
            {width > 0 ? (
              <span style={{ left: liveX, position: "absolute", top: 4, transform: "translateX(-50%)" }}>
                <Text density={density} shout strong textRole="guideMeta" tone="live">
                  Live
                </Text>
              </span>
            ) : null}
          </div>
        ) : null}
      </div>
      {pointer ? edge(SPAN.endMs) : null}
    </div>
  );
};

// A control with its name and (on desktop) its key underneath.
const Labelled = ({ children, keycap, label }: { children: ReactNode; keycap: string; label: string }) => (
  <div style={{ alignItems: "center", display: "flex", flexDirection: "column", gap: 10, width: 128 }}>
    <div style={{ alignItems: "center", display: "flex", height: 76 }}>{children}</div>
    <span style={{ alignItems: "center", display: "inline-flex", gap: 6 }}>
      <span style={{ border: `1px solid ${flat.hairline}`, borderRadius: 4, padding: "0 5px" }}>
        <Text density="pointer" textRole="captionMono" tone="muted">
          {keycap}
        </Text>
      </span>
      <Text density="pointer" textRole="cardMeta" tone="secondary">
        {label}
      </Text>
    </span>
  </div>
);

const UpNext = ({ clock, inBreak }: { clock: string; inBreak: boolean }) => {
  const item = inBreak ? current : next;
  return (
    <div
      style={{
        alignItems: "center",
        background: flat.raised,
        border: `1px solid ${flat.hairline}`,
        borderRadius: 10,
        display: "flex",
        flexShrink: 0,
        gap: 14,
        padding: 8,
        paddingRight: 18,
        width: 340,
      }}
    >
      <div
        style={{
          background: `${flat.elevated} url("${item.stillUri}") center/cover`,
          border: `1px solid ${flat.hairline}`,
          borderRadius: 6,
          flexShrink: 0,
          height: 72,
          width: 128,
        }}
      />
      <div style={{ display: "flex", flexDirection: "column", gap: 2, minWidth: 0 }}>
        <Text density="pointer" shout strong textRole="guideIdent" tone="muted">
          {inBreak ? "Back to" : "Up next"}
        </Text>
        <Text density="pointer" numberOfLines={1} strong textRole="label" tone="primary">
          {item.name}
        </Text>
        <Text density="pointer" numberOfLines={1} textRole="cardMeta" tone="secondary">
          {inBreak
            ? `Ends ${formatTime(current.stopMs)}`
            : `${clock} · ${item.chips.filter((c) => c !== "TV-PG").join(" · ")}`}
        </Text>
      </div>
    </div>
  );
};

const RichChrome = ({ device, scenario, tint, tintFor }: ChromeProps) => {
  const density: Density = device === "desktop" ? "pointer" : "touch";
  const pointer = density === "pointer";
  const landscape = device === "landscape";
  if (scenario.cover) return <Cover density={density} kind={scenario.cover} />;
  const sheet = scenario.sheet;
  const controls = scenario.visible && !sheet;
  const card = scenario.ident && !scenario.visible && !sheet;
  const shown = controls || card;
  const behind = scenario.lagMs > 1000;
  const lag = scenario.lagMs;
  const endsAt = formatTime((scenario.inBreak ? b1.stopMs : current.stopMs) + lag);
  const breakLeft = Math.max(1, Math.round((b1.stopMs - scenario.playheadMs) / MINUTE));
  const runtime = `${Math.round((current.stopMs - current.startMs) / MINUTE)} min`;
  // A break is a badge on the programme, never a title of its own (the review's agreed rule).
  const meta: ReactNode[] = scenario.inBreak
    ? [<BreakBadge density={density} key="b" />, `Back in ${breakLeft} min`]
    : [
        nowMeta.year,
        <RatingBadge density={density} key="r" rating={nowMeta.rating} />,
        runtime,
        nowMeta.genre,
      ];
  // During a break the programme keeps its own lines; the badge alone says it's a break.
  const synopsis = current.synopsis;
  const subtitle = scenario.inBreak ? current.line : scenario.subtitle;
  const side = landscape ? 48 : 52;
  const controlRow = (
    <div
      style={{
        alignItems: pointer ? "flex-start" : "center",
        display: "flex",
        gap: pointer ? 8 : landscape ? 28 : 20,
        justifyContent: "center",
      }}
    >
      {pointer ? (
        <>
          <Labelled keycap="↓" label="Channel down">
            <Tool
              bright
              ringed
              density={density}
              icon="channelDown"
              keyHint="↓"
              label="Channel down"
              size={56}
            />
          </Labelled>
          <Labelled keycap="Space" label={scenario.mode === "paused" ? "Play" : "Pause"}>
            <PlayDisc large playing={scenario.mode !== "paused"} size={72} />
          </Labelled>
          <Labelled keycap="↑" label="Channel up">
            <Tool bright ringed density={density} icon="channelUp" keyHint="↑" label="Channel up" size={56} />
          </Labelled>
          <Labelled keycap="M" label="More">
            <Tool
              bright
              ringed
              density={density}
              icon="more"
              keyHint="M"
              label="More"
              size={56}
              tipAlign="end"
            />
          </Labelled>
        </>
      ) : (
        <>
          <Tool bright ringed density={density} icon="channelDown" label="Channel down" size={side} />
          <PlayDisc large playing={scenario.mode !== "paused"} size={landscape ? 60 : 64} />
          <Tool bright ringed density={density} icon="channelUp" label="Channel up" size={side} />
          <Tool bright ringed density={density} icon="more" label="More" size={side} />
        </>
      )}
    </div>
  );
  const clockBlock = (
    <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
      {behind ? (
        <Text density={density} textRole="label" tone="primary">
          {lagLabel(lag)} behind live
        </Text>
      ) : (
        <Text density={density} strong textRole="label" tone="primary">
          {formatTime(NOW)}
        </Text>
      )}
      <Text density={density} textRole="compact" tone="secondary">
        {scenario.inBreak ? `Back at ${endsAt}` : `Ends ${endsAt}`}
      </Text>
    </div>
  );
  return (
    <>
      {shown ? <div style={scrimStyle(pointer ? 560 : landscape ? 360 : 420, true)} /> : null}
      <div
        aria-hidden={!shown || undefined}
        style={{
          bottom: 0,
          display: "flex",
          flexDirection: "column",
          gap: pointer ? 18 : landscape ? 6 : 10,
          left: 0,
          opacity: shown ? 1 : 0,
          padding: pointer ? "0 40px 32px" : landscape ? "0 20px 10px" : "0 16px 24px",
          pointerEvents: shown ? "auto" : "none",
          position: "absolute",
          right: 0,
        }}
      >
        <div style={{ alignItems: "flex-end", display: "flex", gap: 32, justifyContent: "space-between" }}>
          <div
            style={{ display: "flex", flex: 1, flexDirection: "column", gap: pointer ? 10 : 6, minWidth: 0 }}
          >
            <div style={{ alignItems: "center", display: "flex", gap: 12, justifyContent: "space-between" }}>
              <ChannelChip density={density} tint={tint} />
              {!pointer && !card && lastChannel ? (
                <Tool density={density} label={`Last channel, ${lastChannel.number}`} size={44} swap />
              ) : null}
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
              <Text
                density={density}
                numberOfLines={landscape ? 1 : 2}
                strong
                textRole={pointer ? "display" : "title"}
                tone="primary"
              >
                {scenario.inBreak ? current.name : scenario.title}
              </Text>
              {landscape && !card ? null : (
                <Text
                  density={density}
                  numberOfLines={1}
                  textRole={pointer ? "reading" : "body"}
                  tone="secondary"
                >
                  {card ? stationLine : subtitle}
                </Text>
              )}
            </div>
            {card ? null : (
              <>
                <MetaRow
                  density={density}
                  items={landscape && !scenario.inBreak ? [scenario.subtitle, ...meta] : meta}
                />
                {pointer ? (
                  <div style={{ maxWidth: 680 }}>
                    <Text density={density} numberOfLines={1} textRole="reading" tone="secondary">
                      {synopsis}
                    </Text>
                  </div>
                ) : null}
              </>
            )}
          </div>
          {pointer && !card ? (
            <UpNext clock={formatTime(next.startMs + lag)} inBreak={scenario.inBreak} />
          ) : null}
        </div>
        {card ? null : (
          <>
            <div style={{ display: "flex", flexDirection: "column", gap: pointer ? 0 : 6 }}>
              <Scrubber density={density} pointer={pointer} scenario={scenario} />
              {pointer ? null : (
                <div style={{ alignItems: "center", display: "flex", justifyContent: "space-between" }}>
                  {behind ? (
                    <GoLive density={density} />
                  ) : (
                    <Text density={density} shout strong textRole="guideIdent" tone="live">
                      ● Live
                    </Text>
                  )}
                  <Text density={density} textRole="compact" tone="secondary">
                    {behind ? `${lagLabel(lag)} behind · ` : ""}
                    {scenario.inBreak ? "Back at" : "Ends"} {endsAt}
                  </Text>
                </div>
              )}
            </div>
            {pointer ? (
              <div style={{ alignItems: "center", display: "grid", gridTemplateColumns: "1fr auto 1fr" }}>
                <div style={{ alignItems: "center", display: "flex", gap: 16 }}>
                  {behind ? <GoLive density={density} /> : null}
                  {clockBlock}
                </div>
                {controlRow}
                <div style={{ display: "flex", justifyContent: "flex-end" }}>
                  {lastChannel ? <LastChannel density={density} lastNumber={lastChannel.number} /> : null}
                </div>
              </div>
            ) : (
              controlRow
            )}
          </>
        )}
      </div>
      {sheet === "channels" ? (
        <Sheet density={density} icon="list" title="Channels">
          <ChannelList density={density} tintFor={tintFor} />
        </Sheet>
      ) : sheet === "more" ? (
        <Sheet density={density} icon="more" title="More">
          <MoreBody behind={scenario.mode !== "live"} density={density} />
        </Sheet>
      ) : null}
    </>
  );
};

export { RichChrome };
