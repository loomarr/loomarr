// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// The preview playbar, flattened: no tray, no glow, no ring shadows. A programme is its still with a
// solid label bar; a break is one solid tile with a thumbnail per spot, a hairline between spots.
// Time-true within a block (fifteen minutes behind live to two hours ahead); a break never shrinks
// below a legible tile.
import { Text } from "@loomarr/design-system";
import { useLayoutEffect, useRef, useState } from "react";

import { type Break, formatTime, type Programme, type Segment, WINDOW } from "./data";
import { type Density, flat } from "./flat";

const GAP = 4;
const MIN_BREAK = 40;
const TICKS = [30, 60, 90] as const;

interface Box {
  a: number;
  b: number;
  segment: Segment;
  width: number;
  x: number;
}

const layout = (segments: readonly Segment[], width: number) => {
  const items = segments
    .map((segment) => ({
      a: Math.max(segment.startMs, WINDOW.startMs),
      b: Math.min(segment.stopMs, WINDOW.endMs),
      segment,
    }))
    .filter((i) => i.b > i.a);
  const total = WINDOW.endMs - WINDOW.startMs;
  const avail = width - GAP * (items.length - 1);
  const breakW = (d: number) => Math.max(MIN_BREAK, (avail * d) / total);
  const breaks = items.filter((i) => i.segment.kind === "break");
  const progs = items.filter((i) => i.segment.kind === "programme");
  const progAvail = avail - breaks.reduce((s, i) => s + breakW(i.b - i.a), 0);
  const progDur = progs.reduce((s, i) => s + (i.b - i.a), 0);
  let x = 0;
  const boxes: Box[] = items.map((i) => {
    const w = i.segment.kind === "break" ? breakW(i.b - i.a) : (progAvail * (i.b - i.a)) / progDur;
    const box = { a: i.a, b: i.b, segment: i.segment, width: w, x };
    x += w + GAP;
    return box;
  });
  const xAt = (ms: number) => {
    const t = Math.min(Math.max(ms, WINDOW.startMs), WINDOW.endMs);
    const hit = boxes.find((b) => t >= b.a && t <= b.b);
    if (hit) return hit.x + (hit.width * (t - hit.a)) / (hit.b - hit.a);
    const before = [...boxes].reverse().find((b) => b.b < t);
    return before ? before.x + before.width : 0;
  };
  return { boxes, xAt };
};

const useWidth = () => {
  const ref = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  useLayoutEffect(() => {
    if (!ref.current) return;
    const el = ref.current;
    const measure = () => setWidth(el.clientWidth);
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  return { ref, width };
};

const BreakTile = ({ segment, lit }: { lit: boolean; segment: Break }) => (
  <div
    style={{
      background: flat.infoSurface,
      border: `1px solid ${lit ? flat.info : flat.hairline}`,
      borderRadius: 6,
      display: "flex",
      gap: 1,
      height: "100%",
      padding: 2,
    }}
  >
    {segment.spots.map((spot) => (
      <div
        key={spot.id}
        style={{
          background: `${flat.elevated} url("${spot.still}") center/cover`,
          borderRadius: 3,
          flexBasis: 0,
          flexGrow: spot.durationMs,
        }}
      />
    ))}
  </div>
);

const ProgrammeTile = ({
  density,
  pointer,
  segment,
  width,
}: {
  density: Density;
  pointer: boolean;
  segment: Programme;
  width: number;
}) => (
  <div
    style={{
      background: `${flat.elevated} url("${segment.stillUri}") center/cover`,
      border: `1px solid ${flat.hairline}`,
      borderRadius: 6,
      height: "100%",
      overflow: "hidden",
      position: "relative",
    }}
  >
    {width >= 72 ? (
      <div
        style={{
          alignItems: "baseline",
          background: flat.overlay,
          bottom: 0,
          display: "flex",
          gap: 8,
          justifyContent: "space-between",
          left: 0,
          padding: "3px 8px",
          position: "absolute",
          right: 0,
        }}
      >
        <span
          style={{
            flex: "0 1 auto",
            minWidth: 0,
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
          }}
        >
          <Text density={density} numberOfLines={1} strong textRole="cardMeta" tone="primary">
            {segment.name}
          </Text>
        </span>
        {pointer && width >= 150 ? (
          <Text density={density} textRole="guideMeta" tone="secondary">
            {formatTime(segment.startMs)}
          </Text>
        ) : null}
      </div>
    ) : null}
  </div>
);

interface PlaybarProps {
  density: Density;
  /** Programme or break id drawn as hovered, with its preview card open. */
  hovered?: string;
  playheadMs: number;
  /** Sentence-case labels: "Live", not "LIVE" (variants E and F). */
  plain?: boolean;
  pointer: boolean;
  segments: readonly Segment[];
}

const Playbar = ({ density, hovered, plain = false, playheadMs, pointer, segments }: PlaybarProps) => {
  const { ref, width } = useWidth();
  const { boxes, xAt } = layout(segments, width);
  const head = xAt(Math.min(playheadMs, WINDOW.nowMs));
  const liveX = xAt(WINDOW.nowMs);
  const behind = playheadMs < WINDOW.nowMs - 1000;
  const height = pointer ? 72 : 52;
  return (
    <section aria-label="Schedule">
      <div ref={ref} style={{ height, position: "relative" }}>
        {width > 0
          ? boxes.map((box) => {
              const future = box.segment.startMs > WINDOW.nowMs;
              const isNow = box.segment.startMs <= WINDOW.nowMs && WINDOW.nowMs < box.segment.stopMs;
              return (
                <div
                  key={box.segment.id}
                  style={{
                    filter: future ? playerFuture : undefined,
                    height: "100%",
                    left: box.x,
                    outline: isNow ? `2px solid ${flat.control}` : undefined,
                    outlineOffset: -1,
                    borderRadius: 6,
                    position: "absolute",
                    width: box.width,
                  }}
                >
                  {box.segment.kind === "break" ? (
                    <BreakTile lit={box.segment.id === hovered} segment={box.segment} />
                  ) : (
                    <ProgrammeTile
                      density={density}
                      pointer={pointer}
                      segment={box.segment}
                      width={box.width}
                    />
                  )}
                </div>
              );
            })
          : null}
        {width > 0 ? (
          <>
            <div
              style={{
                background: "rgba(255, 176, 32, 0.12)",
                borderRadius: "6px 0 0 6px",
                bottom: 0,
                left: 0,
                pointerEvents: "none",
                position: "absolute",
                top: 0,
                width: head,
              }}
            />
            <div
              style={{
                background: flat.amber,
                bottom: 0,
                left: head - 1,
                position: "absolute",
                top: -8,
                width: 2,
              }}
            >
              <div
                style={{
                  background: flat.amber,
                  borderRadius: 999,
                  height: 10,
                  left: -4,
                  position: "absolute",
                  top: -4,
                  width: 10,
                }}
              />
            </div>
            {behind ? (
              <div
                style={{
                  background: flat.live,
                  bottom: 0,
                  left: liveX - 1,
                  opacity: 0.8,
                  position: "absolute",
                  top: 0,
                  width: 2,
                }}
              />
            ) : null}
          </>
        ) : null}
      </div>
      <div aria-hidden style={{ height: 16, marginTop: 6, position: "relative" }}>
        {width > 0 ? (
          <>
            <span style={{ left: 0, position: "absolute" }}>
              <Text density={density} textRole="cardMeta" tone="muted">
                −15m
              </Text>
            </span>
            <span style={{ left: liveX, position: "absolute", transform: "translateX(-50%)" }}>
              <Text density={density} shout={!plain} strong textRole="cardMeta" tone="live">
                Live
              </Text>
            </span>
            {pointer
              ? TICKS.map((m) => {
                  const at = WINDOW.nowMs + m * 60_000;
                  return (
                    <span
                      key={m}
                      style={{ left: xAt(at), position: "absolute", transform: "translateX(-50%)" }}
                    >
                      <Text density={density} textRole="guideMeta" tone="muted">
                        {formatTime(at)}
                      </Text>
                    </span>
                  );
                })
              : null}
          </>
        ) : null}
      </div>
    </section>
  );
};

const playerFuture = "brightness(0.72) saturate(0.7)";

export { layout, Playbar, useWidth };
