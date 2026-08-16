import * as channelsApi from "@loomarr/api/endpoints/channels";
import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import type { CycleSlotDTO } from "@loomarr/api/models/cycleSlotDTO";
import type { ExcludedDTO } from "@loomarr/api/models/excludedDTO";
import { ExcludedItemDTOReason } from "@loomarr/api/models/excludedItemDTOReason";
import { unwrap } from "@loomarr/api/unwrap";
import { Clapperboard, Clock, ShieldAlert, Tv } from "lucide-react";
import { useEffect, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Disclosure } from "@/components/ui/disclosure";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";
import { EmptyState, ErrorState } from "../../feedback";
import type { ChannelCyclePreviewProps } from "./channel-cycle-preview.type";

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

const nextWeekdayAt = (weekday: number, hour: number): Date => {
  const d = new Date();
  d.setHours(hour, 0, 0, 0);
  const delta = (weekday - d.getDay() + 7) % 7;
  d.setDate(d.getDate() + (delta === 0 && d.getHours() >= hour ? 7 : delta));
  return d;
};

const nextMonthDayAt = (month: number, day: number, hour: number): Date => {
  const now = new Date();
  let year = now.getFullYear();
  let d = new Date(year, month, day, hour, 0, 0, 0);
  if (d.getTime() < now.getTime()) {
    year += 1;
    d = new Date(year, month, day, hour, 0, 0, 0);
  }
  return d;
};

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
      <li
        className="flex items-center gap-2 py-1.5 text-muted-foreground text-xs"
        aria-label="Commercial break"
      >
        <div className="h-px flex-1 bg-border" />
        <span className="uppercase tracking-wide">Commercial break</span>
        <div className="h-px flex-1 bg-border" />
      </li>
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
        "flex items-center gap-3 rounded-md border border-border px-3 py-2",
        pending ? "border-dashed bg-transparent" : "bg-card",
      )}
    >
      <span className="w-6 shrink-0 text-right font-mono text-muted-foreground text-xs">{index + 1}</span>
      {pending ? (
        <Clock className="size-4 shrink-0 text-static-400" aria-hidden />
      ) : (
        <Tv className="size-4 shrink-0 text-muted-foreground" aria-hidden />
      )}
      <span
        className={cn(
          "flex min-w-0 flex-1 items-baseline gap-1.5 text-sm",
          pending && "text-muted-foreground italic",
        )}
      >
        {showPrefix ? <span className="shrink-0 font-medium">{showPrefix}</span> : null}
        {se ? <span className="shrink-0 font-mono text-muted-foreground text-xs">{se}</span> : null}
        {/* The em dash separates the show/SxxExx from the episode title, only when there's a
            show prefix to separate from — a movie (title == its own name) just shows its title. */}
        {showPrefix ? <span className="shrink-0 text-muted-foreground">—</span> : null}
        <span className="truncate">{episodeTitle}</span>
        {pending ? <span className="shrink-0"> (coming soon)</span> : null}
      </span>
      {slot.part && slot.part > 0 ? <Badge variant="neutral">Part {slot.part}</Badge> : null}
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

// ExcludedPanel — "why isn't X on my channel", finally answerable (#263).
//
// ⚠ This report was computed by every single reconcile and thrown away by every caller. The
// scheduler always knew which titles it refused and why; nothing ever showed anyone. Diagnosing
// one over-ceiling episode previously meant querying the media server by hand.
//
// Collapsed by default: on a healthy channel it is a footnote, and a channel with a tight era
// filter can legitimately refuse dozens of titles — an expanded list would bury the schedule
// this panel exists to show. The COUNTS stay visible in the trigger row so nothing is hidden,
// only folded.
const ExcludedPanel = ({ excluded }: { excluded: ExcludedDTO }) => {
  const items = excluded.items ?? [];
  // Nothing refused ⇒ render nothing at all. An "Excluded (0)" row is noise on the overwhelming
  // majority of channels, and its absence is not ambiguous: the schedule above IS the answer.
  if (items.length === 0) return null;

  const safety = excluded.overCeiling + excluded.unrated;
  return (
    <Disclosure className="rounded-md border border-border">
      <div className="flex items-center gap-2 px-3 py-2">
        <ShieldAlert
          className={cn("size-4 shrink-0", safety > 0 ? "text-signal" : "text-muted-foreground")}
          aria-hidden
        />
        <p className="text-sm">
          <span className="font-medium">
            {items.length} {items.length === 1 ? "title" : "titles"} not airing
          </span>{" "}
          <span className="text-muted-foreground">
            {safety > 0
              ? `— ${safety} held back by the audience ceiling`
              : "— filtered out by this channel's rules"}
          </span>
        </p>
        <Disclosure.Trigger className="ml-auto" label="Show which titles are not airing and why" />
      </div>
      <Disclosure.Panel className="border-border border-t">
        {/* Same scroll treatment as the slot list above: a tight era filter can refuse dozens,
            and the panel must not run off the page. tabIndex + aria-label keep the region
            keyboard-reachable (WCAG 2.1.1) — the rows are non-focusable text. */}
        <ul
          className="scroll-thin flex max-h-64 flex-col divide-y divide-border overflow-y-auto"
          // biome-ignore lint/a11y/noNoninteractiveTabindex: a scrollable region IS interactive — see the slot list's note
          tabIndex={0}
          aria-label="Titles not airing"
        >
          {items.map((item, i) => (
            <li
              // ⚠ The key is NOT `item.key` alone: a per-episode ceiling refusal carries its
              // SERIES key, so several rows legitimately share one. Title disambiguates them
              // (it holds the SxxEyy), and the index closes the remaining gap.
              // biome-ignore lint/suspicious/noArrayIndexKey: disambiguator, not the sole key
              key={`${item.key}-${item.title}-${i}`}
              className="flex flex-col gap-0.5 px-3 py-2"
            >
              <span className="truncate text-sm">{item.title || item.key}</span>
              <span
                className={cn(
                  "text-xs",
                  isSafetyReason(item.reason) ? "text-signal" : "text-muted-foreground",
                )}
              >
                {REASON_LABEL[item.reason]}
              </span>
            </li>
          ))}
        </ul>
      </Disclosure.Panel>
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
    <section className={cn("flex flex-col gap-3", className)}>
      <div>
        <div className="flex items-center gap-2">
          <h3 className="font-semibold text-base">Preview what airs</h3>
          {/* ⚠ Says WHICH schedule is on screen. Without it, a draft preview and a saved one
              are indistinguishable — and an author who has just edited a rule would have no
              way to tell whether they are looking at their change or at what is live. */}
          {draftPolicy ? <Badge variant="neutral">Unsaved changes</Badge> : null}
        </div>
        <p className="text-muted-foreground text-sm">
          {draftPolicy
            ? "What would air if you applied these changes. Time-travel to any moment to check a rule before saving."
            : "Time-travel the schedule. See exactly which rule wins and what plays at any moment, past or future."}
        </p>
      </div>

      <div className="flex flex-wrap items-end gap-3">
        <div className="flex flex-col gap-1">
          <Label htmlFor="cycle-preview-at" className="text-muted-foreground text-xs">
            At
          </Label>
          <Input
            id="cycle-preview-at"
            type="datetime-local"
            className="w-56"
            value={toDatetimeLocalValue(at)}
            onChange={(e) => setAt(fromDatetimeLocalValue(e.target.value))}
          />
        </div>
        <div className="flex flex-wrap gap-1.5 pb-0.5">
          {TIME_PRESETS.map((preset) => (
            <Button key={preset.label} variant="outline" size="sm" onClick={() => setAt(preset.at())}>
              {preset.label}
            </Button>
          ))}
        </div>
      </div>

      {(draftPolicy ? draft.isPending : saved.isLoading) ? (
        <p className="text-muted-foreground text-sm">Loading preview…</p>
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
        <div className="flex flex-col gap-3">
          <div className="flex flex-wrap items-center gap-2 rounded-md border border-border bg-card px-3 py-2.5">
            <Clapperboard className="size-4 shrink-0 text-signal" aria-hidden />
            {body.activeRule.matched ? (
              <p className="text-sm">
                Active rule: <span className="font-medium">{body.activeRule.label}</span>{" "}
                <span className="text-muted-foreground">(priority {body.activeRule.priority})</span>
              </p>
            ) : (
              <p className="text-muted-foreground text-sm">No rule active: base policy</p>
            )}
            <span className="ml-auto font-mono text-muted-foreground text-xs">
              {humanWindow(body.windowMs)}
            </span>
          </div>

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
            <ul
              className="scroll-thin flex max-h-96 flex-col gap-1.5 overflow-y-auto"
              // biome-ignore lint/a11y/noNoninteractiveTabindex: a scrollable region IS interactive — axe's scrollable-region-focusable requires this tabIndex so keyboard users can scroll (see comment above)
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
            </ul>
          )}

          {/* ⚠ BELOW the schedule, deliberately. The question it answers ("why isn't X on?")
              is one you ask after looking at what IS on and finding something missing — put
              first, it would read as an error banner on a channel that is working correctly. */}
          <ExcludedPanel excluded={body.excluded} />
        </div>
      )}
    </section>
  );
};

export { ChannelCyclePreview };
