/** What the lower third says, whatever its state. */
type LowerThirdState =
  | {
      kind: "on";
      title: string;
      episode?: string;
      rating?: string;
      minutesLeft: number;
      next?: LowerThirdNext;
      paused?: boolean;
      /** How far behind live the viewer is; 0 or absent is live. */
      behindSeconds?: number;
    }
  | { kind: "tuning" }
  | { kind: "break"; title: string; backInMinutes: number; next?: LowerThirdNext }
  | { kind: "nothing" }
  /** A number being typed; `target` is the channel it names, absent when there is none. */
  | { kind: "typing"; digits: string; target?: { name: string; onNow?: string } };

interface LowerThirdNext {
  /** A formatted clock time. */
  startsAt: string;
  title: string;
}

const minutes = (count: number) => `${count} ${count === 1 ? "minute" : "minutes"}`;

/** Minutes left or until: part of a minute counts as one, and there is never "0 min". */
const wholeMinutes = (count: number) => Math.max(1, Math.ceil(count));

/** `45 s behind live`, and minutes from a minute on. Both count only what has fully passed. */
const behindWords = (seconds: number) =>
  seconds < 60 ? `${Math.floor(seconds)} s behind live` : `${Math.floor(seconds / 60)} min behind`;

/** The one sentence a screen reader hears: the channel, what is on and how long is left. */
const summaryOf = (channelNumber: number, channelName: string, state: LowerThirdState): string => {
  const channel = `Channel ${channelNumber}, ${channelName}.`;
  const next = "next" in state && state.next ? ` Next at ${state.next.startsAt}, ${state.next.title}.` : "";
  switch (state.kind) {
    case "typing":
      return state.target
        ? `Channel ${state.digits}, ${state.target.name}. Press Enter, or wait.`
        : `There is no channel ${state.digits}.`;
    case "tuning":
      return `${channel} Tuning in.`;
    case "nothing":
      return `${channel} Nothing scheduled.`;
    case "break":
      return `${channel} In a break. ${state.title} is back in ${minutes(wholeMinutes(state.backInMinutes))}.${next}`;
    case "on": {
      const behind = Math.floor(state.behindSeconds ?? 0);
      const where = state.paused
        ? " Paused."
        : behind > 0
          ? ` ${behind < 60 ? `${behind} ${behind === 1 ? "second" : "seconds"}` : minutes(Math.floor(behind / 60))} behind live.`
          : ` ${minutes(wholeMinutes(state.minutesLeft))} left.`;
      const rating = state.rating ? ` Rated ${state.rating}.` : "";
      return `${channel} ${state.title}.${rating}${where}${next}`;
    }
  }
};

export type { LowerThirdNext, LowerThirdState };
export { behindWords, summaryOf, wholeMinutes };
