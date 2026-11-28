import { Layer, Text } from "@loomarr/design-system";

const TICK_MS = 30 * 60_000;
/** A time label this close to the present would run into the word that marks it. */
const CLEAR_MS = 9 * 60_000;

interface RulerProps {
  dayName: ((ms: number) => string | undefined) | undefined;
  formatTime: (ms: number) => string;
  nowMs: number;
  startMs: number;
  stopMs: number;
}

/** The shared clock: a time every half hour, the present marked, and any day but today named. */
const Ruler = ({ dayName, formatTime, nowMs, startMs, stopMs }: RulerProps) => {
  const share = (ms: number) => `${((ms - startMs) / (stopMs - startMs)) * 100}%` as const;
  const ticks: number[] = [];
  for (let at = Math.ceil(startMs / TICK_MS) * TICK_MS; at < stopMs; at += TICK_MS) ticks.push(at);
  const present = nowMs >= startMs && nowMs < stopMs;
  return (
    <Layer height={22} inFlow width="100%">
      {ticks.map((at, index) => {
        // A day is named where the ruler first reaches it.
        const day = dayName?.(at);
        const named = day && (index === 0 || dayName?.(ticks[index - 1] as number) !== day) ? day : undefined;
        if (present && Math.abs(at - nowMs) <= CLEAR_MS && !named) return null;
        return (
          <Layer key={at} left={share(at)} top={2}>
            <Text numberOfLines={1} textRole="mono" tone="secondary">
              {named ? `${named} ${formatTime(at)}` : formatTime(at)}
            </Text>
          </Layer>
        );
      })}
      {present ? (
        <Layer left={share(nowMs)} top={2}>
          <Text textRole="mono" tone="live">
            Now
          </Text>
        </Layer>
      ) : null}
    </Layer>
  );
};

export { Ruler };
