import * as channelsApi from "@loomarr/api/endpoints/channels";
import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import type { CycleSlotDTO } from "@loomarr/api/models/cycleSlotDTO";
import type { ExcludedDTO } from "@loomarr/api/models/excludedDTO";
import { ExcludedItemDTOReason } from "@loomarr/api/models/excludedItemDTOReason";
import { ScheduleFactDTOReason } from "@loomarr/api/models/scheduleFactDTOReason";
import type { ScheduleTraceDTO } from "@loomarr/api/models/scheduleTraceDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { Action, Badge, DateField, Disclosure, Icon, icons, Stack, Text } from "@loomarr/design-system";
import { useEffect, useState } from "react";
import { cn } from "@/lib/utils";
import { EmptyState, ErrorState } from "../../feedback";
import type { ChannelCyclePreviewProps } from "./channel-cycle-preview.type";
import { nextMonthDayAt, nextWeekdayAt } from "./time-presets";

// One import provides both the type and const: orval derives the union from the frozen object,
// so the switchable values and the type that constrains them cannot drift.

// A quick-pick preset: label + a function from "now" to the RFC3339 `at` it produces.
// Kept as pure date math (no server round trip) — the preview endpoint is what actually
// resolves holidays/dayparts, this is just convenient starting points for the picker.
// Matches use-channel-filler-draft's debounce: the two draft previews on this page should
// settle on the same rhythm, or editing filler and editing rules feel like different apps.
const PREVIEW_DEBOUNCE_MS = 300;

interface TimePreset {
  label: string;
  at: () => string;
}

const TIME_PRESETS: TimePreset[] = [
  { label: "Now", at: () => "" },
  { label: "Sat 9am", at: () => nextWeekdayAt(6, 9).toISOString() },
  { label: "Weekday primetime", at: () => nextWeekdayAt(2, 20).toISOString() },
  { label: "Christmas morning", at: () => nextMonthDayAt(11, 25, 9).toISOString() },
  { label: "Halloween night", at: () => nextMonthDayAt(9, 31, 20).toISOString() },
];

// toDatetimeLocalValue / fromDatetimeLocalValue — convert between an RFC3339 `at` string
// and the value a <input type="datetime-local"> wants/produces (no timezone suffix,
// local wall-clock, minute precision). The preview evaluates against the CONTAINER
// wall-clock server-side, so what the picker shows is "local browser time" as a proxy —
// close enough for "what airs Saturday 9am", and the resolved `at` echoed back in the
// response is the authority on what was actually evaluated.
const toDatetimeLocalValue = (at: string): string => {
  if (!at) return "";
  const d = new Date(at);
  if (Number.isNaN(d.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
};

const fromDatetimeLocalValue = (v: string): string => {
  if (!v) return "";
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return "";
  return d.toISOString();
};

// humanWindow — windowMs (0 = whole run) as a short horizon phrase.
const humanWindow = (windowMs: number): string => {
  if (!windowMs || windowMs <= 0) return "Whole run, no rolling window";
  const hours = Math.round(windowMs / (60 * 60 * 1000));
  if (hours < 24) return `Showing ~${hours}h`;
  const days = Math.round(hours / 24);
  return days === 1 ? "Showing ~24h" : `Showing ~${days} days`;
};

// seasonEpisode — "S1E5" from a slot's season/episode (both > 0), else "". A movie or an
// unnumbered item carries 0s and gets no marker.
const seasonEpisode = (slot: CycleSlotDTO): string =>
  slot.season && slot.episode ? `S${slot.season}E${slot.episode}` : "";

// One cycle slot: a program (show · SxxExx — episode), a pending acquisition (muted "coming
// soon"), or a break (a thin divider — Tunarr owns the clip, this is just the gap). `showTitle`
// is the show name resolved from the slot's key by the caller's lineup map (a series episode
// otherwise reads as a bare episode title with no show context on a multi-show channel).
const SlotRow = ({ slot, index, showTitle }: { slot: CycleSlotDTO; index: number; showTitle?: string }) => {
  if (slot.kind === "break") {
    return (
      <Stack
        align="center"
        aria-label="Commercial break"
        className="py-1.5"
        direction="row"
        gap="inline"
        render="li"
      >
        <div className="bg-border h-px flex-1" />
        <Text as="span" textRole="hint" tone="secondary">
          Commercial break
        </Text>
        <div className="bg-border h-px flex-1" />
      </Stack>
    );
  }
  const pending = slot.kind === "pending";
  const se = seasonEpisode(slot);
  const episodeTitle = slot.title || "Untitled";
  // Show the show-name prefix only when it adds information — a series episode ("Bluey" +
  // "Grandad"), not a movie whose lineup name equals its own title ("Shrek" + "Shrek").
  const showPrefix = showTitle && showTitle !== episodeTitle ? showTitle : "";
  return (
    <li
      className={cn(
        "border-border flex items-center gap-3 rounded-md border px-3 py-2",
        pending ? "border-dashed bg-transparent" : "bg-card",
      )}
    >
      <Text as="span" className="w-6 shrink-0 text-right" textRole="mono">
        {index + 1}
      </Text>
      {pending ? (
        <Icon decorative glyph={icons.time} size="compact" tone="muted" />
      ) : (
        <Icon decorative glyph={icons.channels} size="compact" tone="muted" />
      )}
      <Text
        as="span"
        className={cn("flex min-w-0 flex-1 items-baseline gap-1.5", pending && "italic")}
        textRole="compact"
        tone={pending ? "secondary" : "primary"}
      >
        {showPrefix ? (
          <Text as="span" className="shrink-0" textRole="cardLabel" tone="primary">
            {showPrefix}
          </Text>
        ) : null}
        {se ? (
          <Text as="span" className="shrink-0" textRole="mono">
            {se}
          </Text>
        ) : null}
        {/* The em dash separates the show/SxxExx from the episode title, only when there's a
            show prefix to separate from — a movie (title == its own name) just shows its title. */}
        {showPrefix ? (
          <Text as="span" className="shrink-0" textRole="compact" tone="secondary">
            —
          </Text>
        ) : null}
        <span className="truncate">{episodeTitle}</span>
        {pending ? <span className="shrink-0"> (coming soon)</span> : null}
      </Text>
      {slot.part && slot.part > 0 ? (
        <Badge shape="square" tone="neutral">
          Part {slot.part}
        </Badge>
      ) : null}
    </li>
  );
};

// REASON_LABEL — why the scheduler refused an item, in the operator's words.
//
// ⚠ Keyed off the generated `ExcludedItemDTOReason` symbol, not hand-typed strings: `reason` is
// a closed BE enum, so a value added server-side must fail the typecheck here rather than
// silently render as a blank label.
const REASON_LABEL: Record<ExcludedItemDTOReason, string> = {
  [ExcludedItemDTOReason.over_ceiling]: "Rated above this channel's audience ceiling",
  [ExcludedItemDTOReason.unrated]: "No usable rating, and this channel excludes unrated content",
  [ExcludedItemDTOReason.out_of_scope]: "Outside the channel's era, genres, or runtime limits",
  [ExcludedItemDTOReason.out_of_season]: "Benched — outside its seasonal window right now",
};

// A refusal by the AUDIENCE ceiling is styled apart from one by a taste filter. They are not the
// same event: an out-of-scope drop is a curation choice the operator made, while an over-ceiling
// or unrated drop is a safety gate firing, and the operator's likely next action differs
// (loosen the era vs. go fix the rating on the item, or accept it).
const isSafetyReason = (reason: ExcludedItemDTOReason): boolean =>
  reason === ExcludedItemDTOReason.over_ceiling || reason === ExcludedItemDTOReason.unrated;

// ExcludedPanel — "why isn't X on my channel", answered (#263).
//
// ⚠ Every reconcile computes this report: the scheduler knows which titles it refused and why.
// This panel is the only place anyone sees it; without it, diagnosing one over-ceiling episode
// means querying the media server by hand.
//
// Collapsed by default: on a healthy channel it is a footnote, and a channel with a tight era
// filter can legitimately refuse dozens of titles — an expanded list would bury the schedule
// this panel exists to show. The COUNTS stay visible in the trigger row so nothing is hidden,
// only folded.
const ExcludedPanel = ({ excluded }: { excluded: ExcludedDTO }) => {
  const [expanded, setExpanded] = useState(false);
  const items = excluded.items ?? [];
  // Nothing refused ⇒ render nothing at all. An "Excluded (0)" row is noise on the overwhelming
  // majority of channels, and its absence is not ambiguous: the schedule above IS the answer.
  if (items.length === 0) return null;

  const safety = excluded.overCeiling + excluded.unrated;
  return (
    <Disclosure
      label={`${items.length} ${items.length === 1 ? "title" : "titles"} not airing`}
      description={
        safety > 0 ? `${safety} held back by the audience ceiling` : "Filtered out by this channel's rules"
      }
      expanded={expanded}
      onExpandedChange={setExpanded}
    >
      {/* Same scroll treatment as the slot list above: a tight era filter can refuse dozens,
            and the panel must not run off the page. tabIndex + aria-label keep the region
            keyboard-reachable (WCAG 2.1.1) — the rows are non-focusable text. */}
      <Stack
        className="scroll-thin divide-border max-h-64 divide-y overflow-y-auto"
        render="ul"
        tabIndex={0}
        aria-label="Titles not airing"
      >
        {items.map((item, i) => (
          <Stack
            // ⚠ The key is NOT `item.key` alone: a per-episode ceiling refusal carries its
            // SERIES key, so several rows legitimately share one. Title disambiguates them
            // (it holds the SxxEyy), and the index closes the remaining gap.
            // biome-ignore lint/suspicious/noArrayIndexKey: disambiguator, not the sole key
            key={`${item.key}-${item.title}-${i}`}
            className="px-3 py-2"
            gap="0.5"
            render="li"
          >
            <Text as="span" className="truncate" textRole="compact" tone="primary">
              {item.title || item.key}
            </Text>
            <Text as="span" textRole="hint" tone={isSafetyReason(item.reason) ? "warning" : "secondary"}>
              {REASON_LABEL[item.reason]}
            </Text>
          </Stack>
        ))}
      </Stack>
    </Disclosure>
  );
};

// Closed scheduler vocabulary rendered in operator language. This is intentionally keyed by
// the generated enum: adding a backend reason must fail FE typechecking until its explanation is
// written, rather than leaking a machine slug or silently showing a blank row.
const TRACE_REASON_LABEL: Record<ScheduleFactDTOReason, string> = {
  [ScheduleFactDTOReason.eligible]: "Passed the channel's safety and scope filters",
  [ScheduleFactDTOReason.over_ceiling]: "Refused above the audience ceiling",
  [ScheduleFactDTOReason.unrated]: "Refused because no usable rating was available",
  [ScheduleFactDTOReason.out_of_scope]: "Refused by the channel's era, genre, collection, or runtime scope",
  [ScheduleFactDTOReason.out_of_rule_scope]: "Not selected by the rule active at this moment",
  [ScheduleFactDTOReason.out_of_season]: "Benched outside its seasonal window",
  [ScheduleFactDTOReason.available]: "Available in the library",
  [ScheduleFactDTOReason.unavailable_pod_fill]: "Waiting for acquisition; filler holds its place",
  [ScheduleFactDTOReason.unavailable_coming_soon]: "Waiting for acquisition as a coming-soon slot",
  [ScheduleFactDTOReason.complete_selected]: "Included by the complete-series episode policy",
  [ScheduleFactDTOReason.highlights_selected]: "Selected as a series highlight",
  [ScheduleFactDTOReason.highlights_omitted]: "Omitted by the highlights selector",
  [ScheduleFactDTOReason.holiday_selected]: "Selected by the holiday episode policy",
  [ScheduleFactDTOReason.holiday_omitted]: "Omitted by the holiday episode policy",
  [ScheduleFactDTOReason.full_run_fallback]:
    "Kept by the safe full-run fallback because editorial evidence could not narrow the series",
  [ScheduleFactDTOReason.out_of_season_range]: "Outside the approved season range",
  [ScheduleFactDTOReason.sequential]: "Placed in approved sequential order",
  [ScheduleFactDTOReason.shuffle]: "Placed by the deterministic shuffle",
  [ScheduleFactDTOReason.syndication]: "Placed by the deterministic syndication deck",
  [ScheduleFactDTOReason.windowed_out]: "Rotated out of this rolling window",
  [ScheduleFactDTOReason.commercial_break]: "Inserted by the channel's commercial-break cadence",
};

const ScheduleTracePanel = ({ trace }: { trace: ScheduleTraceDTO }) => {
  const [expanded, setExpanded] = useState(false);
  if (trace.recordedTotal === 0 && !trace.truncated) return null;
  const omitted = Math.max(0, trace.factTotal - trace.recordedTotal);
  return (
    <Disclosure
      label={`${trace.recordedTotal} recorded ${trace.recordedTotal === 1 ? "decision" : "decisions"}`}
      description={`${trace.ordering} · trace v${trace.version}`}
      expanded={expanded}
      onExpandedChange={setExpanded}
    >
      {omitted > 0 ? (
        <Text as="p" className="border-border border-b px-3 py-2" textRole="hint" tone="secondary">
          {omitted} additional {omitted === 1 ? "decision" : "decisions"} omitted by the trace bound.
        </Text>
      ) : null}
      <Stack
        className="scroll-thin divide-border max-h-72 divide-y overflow-y-auto"
        render="ul"
        tabIndex={0}
        aria-label="Scheduler decisions"
      >
        {trace.facts.map((fact, i) => (
          <Stack
            // Repeated airings legitimately share key/title/reason; position + index keep rows distinct.
            // biome-ignore lint/suspicious/noArrayIndexKey: index disambiguates repeated trace facts
            key={`${fact.stage}-${fact.key ?? "gap"}-${fact.reason}-${fact.cyclePosition ?? "x"}-${i}`}
            className="px-3 py-2"
            gap="0.5"
            render="li"
          >
            <Text as="span" className="truncate" textRole="compact" tone="primary">
              {fact.title || fact.key || "Commercial gap"}
            </Text>
            <Text as="span" textRole="hint" tone="secondary">
              {TRACE_REASON_LABEL[fact.reason]}
              {fact.cyclePosition !== undefined ? ` · cycle slot ${fact.cyclePosition + 1}` : ""}
            </Text>
          </Stack>
        ))}
      </Stack>
    </Disclosure>
  );
};

// ChannelCyclePreview — the time-travel preview (programming-design §8.1): a pure,
// read-only look at what would air at a chosen wall-clock, computed by the IDENTICAL
// ComputeDesiredAt the reconciler runs. Its whole point is making first-match-by-
// priority legible — "at Saturday 9am, the Weekend TNG marathon rule is active" — so the
// active-rule attribution is the most prominent thing on the panel, not a footnote.
const ChannelCyclePreview = ({ channelId, lineupKeys, draftPolicy, className }: ChannelCyclePreviewProps) => {
  const [at, setAt] = useState("");

  // ⚠ **The draft REPLACES the saved preview while editing, rather than sitting beside it.**
  // Two panes would make the reader diff them; one pane that always answers "what would air
  // if I applied what I am looking at" is the question an author actually has. The saved
  // schedule is still one Apply-or-discard away, and the badge below says which is on screen.
  const saved = channelsApi.usePreviewChannelCycle(channelId, at ? { at } : undefined, {
    query: { enabled: !draftPolicy },
  });
  const draft = channelsApi.usePreviewChannelProgramming();

  // Debounced re-preview whenever the canonical draft settles — the same shape
  // use-channel-filler-draft uses for the filler sandbox, deliberately, rather than a second
  // hand-rolled debounce that could drift from it. Keying on the SERIALIZED policy means a
  // parent re-render, or editing a field back to its old value, does not re-POST.
  const draftKey = draftPolicy ? JSON.stringify(draftPolicy) : "";
  // biome-ignore lint/correctness/useExhaustiveDependencies: draftKey + at ARE the dependencies
  useEffect(() => {
    if (!draftKey) return;
    const t = setTimeout(() => {
      draft.mutate({
        id: channelId,
        params: at ? { at } : undefined,
        data: { policy: JSON.parse(draftKey) as ChannelPolicy },
      });
    }, PREVIEW_DEBOUNCE_MS);
    return () => clearTimeout(t);
  }, [channelId, draftKey, at]);

  // One source, chosen once: every consumer below reads `body`, `isLoading` and `error` and
  // never asks which endpoint produced them.
  const active = draftPolicy ? draft : saved;
  const body = unwrap(active.data);

  // key → show title, from the channel's lineup, so each episode slot names its show.
  const showByKey = new Map((lineupKeys ?? []).map((e) => [e.key, e.title]));

  return (
    <Stack className={className} gap="control" render="section">
      <div>
        <Stack align="center" direction="row" gap="inline">
          <Text as="h3" textRole="heading">
            Preview what airs
          </Text>
          {/* ⚠ Says WHICH schedule is on screen. Without it, a draft preview and a saved one
              are indistinguishable — and an author who has just edited a rule would have no
              way to tell whether they are looking at their change or at what is live. */}
          {draftPolicy ? (
            <Badge shape="square" tone="neutral">
              Unsaved changes
            </Badge>
          ) : null}
        </Stack>
        <Text as="p" textRole="compact" tone="secondary">
          {draftPolicy
            ? "What would air if you applied these changes. Time-travel to any moment to check a rule before saving."
            : "Time-travel the schedule. See exactly which rule wins and what plays at any moment, past or future."}
        </Text>
      </div>

      <Stack align="end" direction="row" gap="control" wrap>
        <DateField
          id="cycle-preview-at"
          kind="datetime"
          label="At"
          onValueChange={(value) => setAt(fromDatetimeLocalValue(value))}
          value={toDatetimeLocalValue(at)}
          width={224}
        />
        <Stack className="pb-0.5" direction="row" gap="1.5" wrap>
          {TIME_PRESETS.map((preset) => (
            <Action key={preset.label} onPress={() => setAt(preset.at())} variant="outline">
              {preset.label}
            </Action>
          ))}
        </Stack>
      </Stack>

      {(draftPolicy ? draft.isPending : saved.isLoading) ? (
        <Text as="p" textRole="compact" tone="secondary">
          Loading preview…
        </Text>
      ) : active.error || !body ? (
        // ⚠ Retry differs by source: the saved preview is a QUERY (refetch), the draft is a
        // MUTATION (re-POST the current draft). Calling refetch on the mutation would be a
        // runtime TypeError, so the branch is explicit rather than clever.
        <ErrorState
          error={active.error}
          onRetry={() =>
            draftPolicy
              ? draft.mutate({
                  id: channelId,
                  params: at ? { at } : undefined,
                  data: { policy: draftPolicy },
                })
              : void saved.refetch()
          }
        />
      ) : (
        <Stack gap="control">
          <Stack
            align="center"
            className="border-border bg-card rounded-md border px-3 py-2.5"
            direction="row"
            gap="inline"
            wrap
          >
            <Icon decorative glyph={icons.filler} size="compact" tone="muted" />
            {body.activeRule.matched ? (
              <Text as="p" textRole="compact" tone="primary">
                Active rule:{" "}
                <Text as="span" textRole="cardLabel" tone="primary">
                  {body.activeRule.label}
                </Text>{" "}
                <Text as="span" textRole="compact" tone="secondary">
                  (priority {body.activeRule.priority})
                </Text>
              </Text>
            ) : (
              <Text as="p" textRole="compact" tone="secondary">
                No rule active: base policy
              </Text>
            )}
            <Text as="span" className="ml-auto" textRole="mono">
              {humanWindow(body.windowMs)}
            </Text>
          </Stack>

          {!body.slots || body.slots.length === 0 ? (
            <EmptyState title="Nothing scheduled" description="No slots resolved for this moment yet." />
          ) : (
            // A long cycle (a whole-run marathon resolves to 50 slots) would run off the
            // page; cap the list to a comfortable window and let it scroll INSIDE. max-height
            // means a short cycle isn't boxed — the scroll only appears once it's actually long.
            // scroll-thin (styles.css) is the token-keyed slim scrollbar (thin thumb, no
            // reserved gutter — short lists render unshifted; see the utility's note).
            // tabIndex + aria-label make the scroll region keyboard-reachable (WCAG 2.1.1,
            // scrollable-region-focusable): rows are non-focusable text, so without a Tab stop
            // on the region itself a keyboard user couldn't scroll past the fold. NOTE: no
            // role override — the <ul> keeps its implicit role="list" (a role="group" would
            // orphan the <li> children and trip the `listitem` rule); aria-label names the
            // list without changing that role.
            <Stack
              className="scroll-thin max-h-96 overflow-y-auto"
              gap="1.5"
              render="ul"
              tabIndex={0}
              aria-label="Scheduled slots"
            >
              {body.slots.map((slot, i) => (
                <SlotRow
                  // The index is only a disambiguator here (multiple breaks, or the same key
                  // airing twice in a marathon share a `kind`+`key`) — position is otherwise
                  // stable within a single preview response, which never reorders in place.
                  // biome-ignore lint/suspicious/noArrayIndexKey: disambiguator, not the sole key
                  key={`${slot.kind}-${slot.key ?? ""}-${i}`}
                  slot={slot}
                  index={i}
                  showTitle={slot.key ? showByKey.get(slot.key) : undefined}
                />
              ))}
            </Stack>
          )}

          {/* ⚠ BELOW the schedule, deliberately. The question it answers ("why isn't X on?")
              is one you ask after looking at what IS on and finding something missing — put
              first, it would read as an error banner on a channel that is working correctly. */}
          <ExcludedPanel excluded={body.excluded} />
          <ScheduleTracePanel trace={body.trace} />
        </Stack>
      )}
    </Stack>
  );
};

export { ChannelCyclePreview };
