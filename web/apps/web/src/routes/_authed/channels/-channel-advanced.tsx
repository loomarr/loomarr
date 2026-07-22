import type { ChannelDTO } from "@loomarr/api";
import { humanizeRelaxation } from "@loomarr/core";

// relaxationSentence — a single eased scheduling rule (a relaxation-ladder step) written as
// a warm, plain sentence rather than a `KIND: from → to` code. Each kind gets its own
// phrasing because "loosened" means something different for each: a no-repeat window shrank,
// a gap disappeared, a per-block cap lifted. Falls back to the humanized label + values for
// any kind without a bespoke sentence, so a new ladder step never renders as raw JSON.
const relaxationSentence = (step: { kind: string; from: string; to: string }): string => {
  const { label, from, to } = humanizeRelaxation(step);
  switch (step.kind) {
    case "episodeNoRepeat":
      return `Episodes can come back around a bit sooner — after ${to}, not ${from}.`;
    case "movieNoRepeat":
      return `Movies can come back around a bit sooner — after ${to}, not ${from}.`;
    case "seriesMinGap":
      return to === "none"
        ? `A series can play a few in a row now (it used to wait ${from}).`
        : `A series waits less between episodes now — ${to} instead of ${from}.`;
    case "blockMax":
      return `No cap on how many episodes run back-to-back (was ${from}).`;
    default:
      return `${label}: ${from} → ${to}.`;
  }
};

// ChannelAdvanced — the scheduler internals, shown only inside the admin-only "Advanced"
// disclosure on the channel-detail page. This is deliberately the place for the things a
// VIEWER should never see: which programming rules Loomarr had to ease to fill the channel,
// and the Tunarr id. Keeping them here is what lets the page above speak in plain language.
// Everything is explained in a sentence, not left as a code. (The commercial-break preview
// moved up into the channel's Filler section — it's now part of choosing filler, not a
// scheduler internal.)
const ChannelAdvanced = ({ channel }: { channel: ChannelDTO }) => {
  const applied = channel.policy?.applied ?? [];

  return (
    <div className="flex flex-col gap-5 border-border border-t px-4 py-4">
      {/* Programming rules that were eased (the relaxation ladder). */}
      <section className="flex flex-col gap-2">
        <h3 className="font-medium text-sm">Programming rules</h3>
        {applied.length > 0 ? (
          <>
            <p className="text-muted-foreground text-sm">
              A few rules Loomarr relaxed so the channel always has something to play:
            </p>
            <ul className="flex flex-col gap-1.5">
              {applied.map((step) => (
                <li key={`${step.kind}:${step.from}->${step.to}`} className="flex gap-2 text-caution text-sm">
                  <span aria-hidden>•</span>
                  <span>{relaxationSentence(step)}</span>
                </li>
              ))}
            </ul>
          </>
        ) : (
          <p className="text-muted-foreground text-sm">
            Running exactly as specified — nothing had to be eased.
          </p>
        )}
      </section>

      {/* The Tunarr link — pure plumbing, useful only for debugging. */}
      <section className="flex flex-col gap-1">
        <h3 className="font-medium text-sm">Broadcast link</h3>
        {channel.tunarrId ? (
          <p className="font-mono text-muted-foreground text-xs">Tunarr channel: {channel.tunarrId}</p>
        ) : (
          <p className="text-muted-foreground text-sm">
            Not on Tunarr yet — it's created there automatically once Tunarr is connected.
          </p>
        )}
      </section>
    </div>
  );
};

export { ChannelAdvanced };
