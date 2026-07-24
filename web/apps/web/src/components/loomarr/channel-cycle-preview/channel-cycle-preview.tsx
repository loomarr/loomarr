import { type CycleSlotDTO, channelsApi } from "@loomarr/api";
import { Clapperboard, Clock, Tv } from "lucide-react";
import { useState } from "react";
import { Badge, Button, Input, Label } from "@/components/ui";
import { cn } from "@/lib";
import { EmptyState } from "../empty-state";
import { ErrorState } from "../error-state";
import type { ChannelCyclePreviewProps } from "./channel-cycle-preview.type";

// A quick-pick preset: label + a function from "now" to the RFC3339 `at` it produces.
// Kept as pure date math (no server round trip) — the preview endpoint is what actually
// resolves holidays/dayparts, this is just convenient starting points for the picker.
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
  if (!windowMs || windowMs <= 0) return "Whole run — no rolling window";
  const hours = Math.round(windowMs / (60 * 60 * 1000));
  if (hours < 24) return `Showing ~${hours}h`;
  const days = Math.round(hours / 24);
  return days === 1 ? "Showing ~24h" : `Showing ~${days} days`;
};

// One cycle slot: a program (title + optional part-N note), a pending acquisition
// (muted "coming soon"), or a break (a thin divider — Tunarr owns the clip, this is
// just the gap).
const SlotRow = ({ slot, index }: { slot: CycleSlotDTO; index: number }) => {
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
      <span className={cn("min-w-0 flex-1 truncate text-sm", pending && "text-muted-foreground italic")}>
        {slot.title || "Untitled"}
        {pending ? " (coming soon)" : ""}
      </span>
      {slot.part && slot.part > 0 ? <Badge variant="neutral">Part {slot.part}</Badge> : null}
    </li>
  );
};

// ChannelCyclePreview — the time-travel preview (programming-design §8.1): a pure,
// read-only look at what would air at a chosen wall-clock, computed by the IDENTICAL
// ComputeDesiredAt the reconciler runs. Its whole point is making first-match-by-
// priority legible — "at Saturday 9am, the Weekend TNG marathon rule is active" — so the
// active-rule attribution is the most prominent thing on the panel, not a footnote.
const ChannelCyclePreview = ({ channelId, className }: ChannelCyclePreviewProps) => {
  const [at, setAt] = useState("");

  const preview = channelsApi.usePreviewChannelCycle(channelId, at ? { at } : undefined);

  const body = preview.data?.status === 200 ? preview.data.data : undefined;

  return (
    <section className={cn("flex flex-col gap-3", className)}>
      <div>
        <h3 className="font-semibold text-base">Preview what airs</h3>
        <p className="text-muted-foreground text-sm">
          Time-travel the schedule — see exactly which rule wins and what plays at any moment, past or future.
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

      {preview.isLoading ? (
        <p className="text-muted-foreground text-sm">Loading preview…</p>
      ) : preview.error || !body ? (
        <ErrorState error={preview.error} onRetry={() => preview.refetch()} />
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
              <p className="text-muted-foreground text-sm">No rule active — base policy</p>
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
                // The index is only a disambiguator here (multiple breaks, or the same key
                // airing twice in a marathon share a `kind`+`key`) — position is otherwise
                // stable within a single preview response, which never reorders in place.
                // biome-ignore lint/suspicious/noArrayIndexKey: disambiguator, not the sole key
                <SlotRow key={`${slot.kind}-${slot.key ?? ""}-${i}`} slot={slot} index={i} />
              ))}
            </ul>
          )}
        </div>
      )}
    </section>
  );
};

export { ChannelCyclePreview };
