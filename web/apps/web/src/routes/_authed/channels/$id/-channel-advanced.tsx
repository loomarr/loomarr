import type { ChannelDTO, ChannelPolicy } from "@loomarr/api";
import { humanizeRelaxation } from "@loomarr/core";
import { Label, Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui";

// relaxationSentence — a single eased scheduling rule (a relaxation-ladder step) written as
// a warm, plain sentence rather than a `KIND: from → to` code. Each kind gets its own
// phrasing because "loosened" means something different for each: a no-repeat window shrank,
// a gap disappeared, a per-block cap lifted. Falls back to the humanized label + values for
// any kind without a bespoke sentence, so a new ladder step never renders as raw JSON.
const relaxationSentence = (step: { kind: string; from: string; to: string }): string => {
  const { label, from, to } = humanizeRelaxation(step);
  switch (step.kind) {
    case "episodeNoRepeat":
      return `Episodes can come back around a bit sooner, after ${to}, not ${from}.`;
    case "movieNoRepeat":
      return `Movies can come back around a bit sooner, after ${to}, not ${from}.`;
    case "seriesMinGap":
      return to === "none"
        ? `A series can play a few in a row now (it used to wait ${from}).`
        : `A series waits less between episodes now, ${to} instead of ${from}.`;
    case "blockMax":
      return `No cap on how many episodes run back-to-back (was ${from}).`;
    default:
      return `${label}: ${from} → ${to}.`;
  }
};

// The per-channel playout override (§9.1). "" = inherit the global `playout.backend` registry
// setting — the nil-means-inherit shape that makes "changing the default affects new channels
// only" true rather than aspirational: a channel that never opted in has no stored value to
// change, so a fleet-wide flip is not expressible by accident.
const BACKEND_OPTIONS: { value: string; label: string }[] = [
  { value: "inherit", label: "Follow the default" },
  { value: "internal", label: "Loomarr" },
  { value: "tunarr", label: "Tunarr" },
];

// ChannelAdvanced — the scheduler internals + broadcast plumbing, shown only inside the
// admin-only "Advanced" disclosure on the channel-detail page. This is deliberately the place
// for the things a VIEWER should never see: which programming rules Loomarr had to ease to
// fill the channel, and who streams it. Keeping them here is what lets the page above speak in
// plain language. Everything is explained in a sentence, not left as a code. (The
// commercial-break preview moved up into the channel's Filler section — it's now part of
// choosing filler, not a scheduler internal.)
//
// Mostly read-only STATUS, with one exception: the playout backend (§9.1 — "one can be moved
// from its own page"). It sits beside the Tunarr link because they are the same subject — who
// streams this channel — and that link's meaning depends on the answer. `onPolicyChange` is
// optional so the diagnostics view stays renderable read-only where no admin handler exists.
const ChannelAdvanced = ({
  channel,
  onPolicyChange,
}: {
  channel: ChannelDTO;
  onPolicyChange?: (next: ChannelPolicy) => void;
}) => {
  const applied = channel.policy?.applied ?? [];
  const policy = channel.policy ?? {};
  const backend = policy.playout?.backend || "inherit";

  return (
    // Just the body content — the container (border + padding) is provided by the
    // CollapsibleSection this renders inside on the channel page.
    <div className="flex flex-col gap-5">
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
            Running exactly as specified. Nothing had to be eased.
          </p>
        )}
      </section>

      {/* Who streams this channel, and the Tunarr link — the same subject. */}
      <section className="flex flex-col gap-2">
        <h3 className="font-medium text-sm">Broadcast</h3>

        {/* Playout backend — orphaned until now: §9.1 promised a channel "can be moved from
            its own page" and no page offered the move, so the per-channel override existed
            only for a hand-written policy_json. */}
        {onPolicyChange && (
          <div className="flex flex-col gap-1">
            <Label htmlFor="channel-playout-backend" className="text-muted-foreground text-xs">
              Streamed by
            </Label>
            <Select
              value={backend}
              onValueChange={(v) =>
                onPolicyChange({
                  ...policy,
                  playout: { ...policy.playout, backend: v === "inherit" ? "" : v },
                })
              }
            >
              <SelectTrigger id="channel-playout-backend" className="w-56">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {BACKEND_OPTIONS.map((o) => (
                  <SelectItem key={o.value} value={o.value}>
                    {o.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {/* Resolved per request at tune time (api/playout.go playsInternally), so this
                does not interrupt a stream already playing — it applies the next time someone
                tunes in. Said plainly because the opposite assumption would make an operator
                avoid a safe change mid-evening. */}
            <p className="text-muted-foreground text-xs">
              Applies the next time someone tunes in. Anyone watching now keeps their stream.
            </p>
          </div>
        )}

        {channel.tunarrId ? (
          <p className="font-mono text-muted-foreground text-xs">Tunarr channel: {channel.tunarrId}</p>
        ) : (
          <p className="text-muted-foreground text-sm">
            Not on Tunarr yet. It's created there automatically once Tunarr is connected.
          </p>
        )}
      </section>
    </div>
  );
};

export { ChannelAdvanced };
