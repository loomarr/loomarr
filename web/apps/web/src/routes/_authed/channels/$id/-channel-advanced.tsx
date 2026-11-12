import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import { humanizeRelaxation } from "@loomarr/core/format";
import { CompactSelect, Stack, Text } from "@loomarr/design-system";

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

// The per-channel playout override (§9.1). "" follows the live global
// `playout.backend`; an explicit value stays pinned across global changes.
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
// commercial-break preview is in the channel's Filler section, not here — it's part of choosing
// filler, not a scheduler internal.)
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
    // Disclosure this renders inside on the channel page. The web mock draws it as a
    // label | value grid. Its "Last updated · took" row has no field on ChannelDTO yet, so it
    // isn't drawn rather than faked.
    <div className="grid grid-cols-[180px_1fr] gap-x-4 gap-y-2">
      {/* Programming rules that were eased (the relaxation ladder). */}
      <Text as="span" textRole="compact" tone="secondary">
        Rules loosened
      </Text>
      {applied.length > 0 ? (
        <Stack gap="1.5" render="ul">
          {applied.map((step) => (
            <li key={`${step.kind}:${step.from}->${step.to}`}>
              <Text as="span" textRole="compact">
                {relaxationSentence(step)}
              </Text>
            </li>
          ))}
        </Stack>
      ) : (
        <Text as="span" textRole="compact">
          Running exactly as specified. Nothing had to be eased.
        </Text>
      )}

      {/* Playout backend — §9.1 says a channel "can be moved from its own page", and this is
          that page: without it the per-channel override exists only for a hand-written
          policy_json. The mock doesn't draw it; it stays because it's the only place the
          override can be set. It sits beside the Tunarr row: the same subject. */}
      {onPolicyChange && (
        <>
          <Text
            as="span"
            className="self-start pt-2"
            id="channel-playout-backend-label"
            textRole="compact"
            tone="secondary"
          >
            Streamed by
          </Text>
          <Stack gap="1">
            <CompactSelect
              aria-labelledby="channel-playout-backend-label"
              id="channel-playout-backend"
              label="Streamed by"
              onValueChange={(v) =>
                onPolicyChange({
                  ...policy,
                  playout: { ...policy.playout, backend: v === "inherit" ? "" : v },
                })
              }
              options={BACKEND_OPTIONS}
              value={backend}
              width={224}
            />
            {/* Resolved per request at tune time (api/playout.go playsInternally), so this
                does not interrupt a stream already playing — it applies the next time someone
                tunes in. Said plainly because the opposite assumption would make an operator
                avoid a safe change mid-evening. */}
            <Text as="p" textRole="hint" tone="secondary">
              Applies the next time someone tunes in. Anyone watching now keeps their stream.
            </Text>
          </Stack>
        </>
      )}

      <Text as="span" textRole="compact" tone="secondary">
        Tunarr channel
      </Text>
      <Text as="span" textRole="mono">
        {channel.tunarrId
          ? channel.tunarrId
          : channel.inAppPlayable
            ? "Not used — Loomarr plays this channel itself"
            : "Not on Tunarr yet. It's created there automatically once Tunarr is connected."}
      </Text>
    </div>
  );
};

export { ChannelAdvanced };
