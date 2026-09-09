import type { Range } from "@loomarr/api/models/range";
import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { FieldHelp } from "../../feedback";
import type { ChannelPolicyFieldsProps } from "./channel-policy-fields.type";

const YEAR_MIN = 1900;
const YEAR_MAX = 2099;

const isValidRange = (range: Range): range is Range & { from: number; to: number } =>
  Number.isInteger(range.from) &&
  Number.isInteger(range.to) &&
  range.from !== undefined &&
  range.to !== undefined &&
  range.from >= YEAR_MIN &&
  range.from <= YEAR_MAX &&
  range.to >= YEAR_MIN &&
  range.to <= YEAR_MAX &&
  range.from <= range.to;

const normalizedRanges = (ranges: Range[] | undefined): Range[] => {
  const valid = (ranges ?? []).filter(isValidRange).sort((a, b) => a.from - b.from || a.to - b.to);
  return valid.reduce<Range[]>((merged, range) => {
    const previous = merged.at(-1);
    if (previous?.to !== undefined && range.from <= previous.to + 1)
      previous.to = Math.max(previous.to, range.to);
    else merged.push({ from: range.from, to: range.to });
    return merged;
  }, []);
};

const DateAxisRow = ({
  label,
  range,
  index,
  onCommit,
  onRemove,
}: {
  label: string;
  range: Range;
  index: number;
  onCommit: (index: number, next: Range) => void;
  onRemove: (index: number) => void;
}) => {
  const [draft, setDraft] = useState({ from: String(range.from), to: String(range.to) });

  // An invalid endpoint remains visible until its partner makes the pair valid. The actual
  // committed range is the reset boundary, so drafts neither survive an external replacement
  // nor slide onto another row when the list changes.
  useEffect(() => {
    setDraft({ from: String(range.from), to: String(range.to) });
  }, [range.from, range.to]);

  return (
    <div className="flex flex-wrap items-end gap-2">
      <div className="flex flex-col gap-1">
        <Label htmlFor={`policy-dates-${label}-${index}-from`} className="text-muted-foreground text-xs">
          From year
        </Label>
        <Input
          id={`policy-dates-${label}-${index}-from`}
          type="number"
          min={YEAR_MIN}
          max={YEAR_MAX}
          className="w-28"
          value={draft.from}
          onChange={(event) => setDraft({ ...draft, from: event.target.value })}
          onBlur={() => onCommit(index, { from: Number(draft.from), to: Number(draft.to) })}
        />
      </div>
      <div className="flex flex-col gap-1">
        <Label htmlFor={`policy-dates-${label}-${index}-to`} className="text-muted-foreground text-xs">
          To year
        </Label>
        <Input
          id={`policy-dates-${label}-${index}-to`}
          type="number"
          min={YEAR_MIN}
          max={YEAR_MAX}
          className="w-28"
          value={draft.to}
          onChange={(event) => setDraft({ ...draft, to: event.target.value })}
          onBlur={() => onCommit(index, { from: Number(draft.from), to: Number(draft.to) })}
        />
      </div>
      <Button type="button" variant="ghost" size="sm" onClick={() => onRemove(index)}>
        Remove
      </Button>
    </div>
  );
};

const DateAxisEditor = ({
  label,
  ranges,
  onChange,
}: {
  label: string;
  ranges: Range[];
  onChange: (next: Range[] | undefined) => void;
}) => {
  const [draft, setDraft] = useState({ from: "", to: "" });
  const [error, setError] = useState<string>();
  const commit = (index: number, next: Range) => {
    if (!isValidRange(next)) {
      setError(`Enter whole years from ${YEAR_MIN} to ${YEAR_MAX}, with From no later than To.`);
      return;
    }
    setError(undefined);
    const changed = [...ranges];
    changed[index] = next;
    onChange(changed);
  };
  const add = () => {
    const from = Number(draft.from);
    const to = Number(draft.to);
    if (
      !draft.from ||
      !draft.to ||
      !Number.isInteger(from) ||
      !Number.isInteger(to) ||
      from < YEAR_MIN ||
      from > YEAR_MAX ||
      to < YEAR_MIN ||
      to > YEAR_MAX ||
      from > to
    ) {
      setError(`Enter whole years from ${YEAR_MIN} to ${YEAR_MAX}, with From no later than To.`);
      return;
    }
    setError(undefined);
    setDraft({ from: "", to: "" });
    onChange([...ranges, { from, to }]);
  };
  return (
    <div className="flex flex-col gap-2 rounded-md border border-border p-3">
      <div className="flex items-center justify-between gap-3">
        <Label className="text-sm">{label}</Label>
      </div>
      {ranges.length === 0 ? (
        <p className="text-muted-foreground text-xs">No date requirement.</p>
      ) : (
        ranges.map((range, index) => (
          <DateAxisRow
            key={`${range.from}-${range.to}`}
            label={label}
            range={range}
            index={index}
            onCommit={commit}
            onRemove={(row) => onChange(ranges.filter((_, current) => current !== row))}
          />
        ))
      )}
      <div className="flex flex-wrap items-end gap-2">
        <Input
          aria-label={`${label} new range from`}
          type="number"
          min={YEAR_MIN}
          max={YEAR_MAX}
          className="w-28"
          value={draft.from}
          onChange={(event) => setDraft({ ...draft, from: event.target.value })}
          placeholder="From"
        />
        <Input
          aria-label={`${label} new range to`}
          type="number"
          min={YEAR_MIN}
          max={YEAR_MAX}
          className="w-28"
          value={draft.to}
          onChange={(event) => setDraft({ ...draft, to: event.target.value })}
          placeholder="To"
        />
        <Button type="button" variant="outline" size="sm" onClick={add} aria-label={`Add ${label} range`}>
          Add range
        </Button>
      </div>
      {error && (
        <p className="text-onair-300 text-xs" role="alert">
          {error}
        </p>
      )}
    </div>
  );
};

// FieldLabel — a form label paired with an (i) help icon (FieldHelp), the row that replaces
// the old permanent helper `<p>` under every control. Keeps the form compact: the guidance is
// on hover, not always on screen. `htmlFor` ties the label to its control; the help names the
// same field for screen readers.
const FieldLabel = ({ htmlFor, children, help }: { htmlFor?: string; children: string; help: string }) => (
  <div className="flex items-center gap-1.5">
    <Label htmlFor={htmlFor}>{children}</Label>
    <FieldHelp label={children}>{help}</FieldHelp>
  </div>
);

// ChannelPolicyFields — a ChannelPolicy as plain-language editable chips (programming-design
// §8). Controlled: the parent holds `policy` and persists whatever this calls `onChange`
// with. `applied` is reconcile-owned (§9) and never rendered here — it is read-only history
// of what the scheduler had to ease, not a setting an operator edits. A field left blank
// clears the authored override; its effective behavior is field-specific and named below.

// A no-repeat window is a Go-duration STRING on the wire ("168h", "30h0m0s"). For display
// we tidy the zero units the backend's Duration.String() emits: "30h0m0s" → "30h", "0s" →
// "" (no restriction). Non-duration text is left as-typed so a bad value is visible, not
// silently dropped.
const tidyDuration = (v: string | undefined): string => {
  if (!v || v === "0s") return "";
  const m = v.match(/^(\d+h)?(\d+m)?(\d+s)?$/);
  if (!m || (!m[1] && !m[2] && !m[3])) return v;
  const trimmed = [m[1], m[2], m[3]].filter((u) => u && !/^0[hms]$/.test(u)).join("");
  return trimmed || v;
};

// On save: pass the typed duration straight through (the backend accepts "24h"/"168h"),
// or undefined for blank — undefined returns to Loomarr's built-in spacing. Whitespace-only
// is treated as blank.
const durationOrUndefined = (v: string): string | undefined => {
  const trimmed = v.trim();
  return trimmed === "" ? undefined : trimmed;
};

const STRATEGY_LABELS: Record<string, string> = {
  sequential: "In order",
  shuffle: "Shuffled",
  time_slot: "Time slots",
};

// Channel.Strategy is a stored create-time fallback, not a second operator-facing order knob.
// Name its resolved value inside the one policy control so "inherit" never asks the operator to
// remember which similarly-named setting it refers to.
const orderingOptions = (strategy?: string): { value: string; label: string }[] => [
  {
    value: "inherit",
    label: strategy
      ? `Use channel strategy (${STRATEGY_LABELS[strategy] ?? strategy})`
      : "Use channel strategy",
  },
  { value: "sequential", label: "In order" },
  { value: "shuffle", label: "Shuffled" },
  { value: "syndication", label: "Syndication" },
];

// The rating ladder, mirroring schedule/policy.go's `ladderRank`. Mirrored ONLY so the
// "Automatic" unrated option can say which way it currently resolves; the gate itself is
// enforced in Go and never relaxed by the relaxation ladder (§8). Kept as ranks rather than a
// hand-copied set of "kids ratings" so the boundary stays one number to compare against —
// KIDS_CEILING_RANK is `kidsCeilingRank` (TV-PG = 3), and "kids" means rank <= that.
const LADDER_RANK: Record<string, number> = {
  "TV-Y": 0,
  "TV-Y7": 1,
  "TV-G": 2,
  G: 2,
  "TV-PG": 3,
  PG: 3,
  "TV-14": 4,
  "PG-13": 4,
  "TV-MA": 5,
  R: 5,
  "NC-17": 5,
};
const KIDS_CEILING_RANK = 3;

// How a `default` unrated policy resolves right now, given the ceiling above it (Go's
// resolveUnrated): a kids ceiling fails closed and skips unrated titles; anything else —
// including no ceiling at all — allows them.
const unratedResolvesToExclude = (ceiling: string | undefined): boolean =>
  ceiling !== undefined &&
  ceiling !== "" &&
  (LADDER_RANK[ceiling] ?? Number.POSITIVE_INFINITY) <= KIDS_CEILING_RANK;

const UNRATED_OPTIONS: { value: string; label: string }[] = [
  { value: "default", label: "Automatic" },
  { value: "exclude", label: "Skip unrated" },
  { value: "allow", label: "Allow unrated" },
];

const CEILING_OPTIONS: { value: string; label: string }[] = [
  { value: "none", label: "No limit" },
  { value: "TV-Y", label: "TV-Y" },
  { value: "TV-Y7", label: "TV-Y7" },
  { value: "TV-G", label: "TV-G" },
  { value: "TV-PG", label: "TV-PG" },
  { value: "TV-14", label: "TV-14" },
  { value: "TV-MA", label: "TV-MA" },
  { value: "G", label: "G" },
  { value: "PG", label: "PG" },
  { value: "PG-13", label: "PG-13" },
  { value: "R", label: "R" },
];

const ChannelPolicyFields = ({ policy, onChange, className, show, strategy }: ChannelPolicyFieldsProps) => {
  const era = policy.scope?.era;
  const dates = policy.scope?.dates;
  const separation = policy.separation;
  // Split for the Programming surface's blocks (§12): scope = audience ceiling + era ("What
  // plays"); ordering = ordering + no-repeat ("How it's ordered"). Omitted = show everything.
  const showScope = show !== "ordering";
  const showOrdering = show !== "scope";

  return (
    // A responsive 2-column field grid (was a 1-wide stack): the two Selects sit side by side,
    // while Era + No-repeat span the full width because they each hold a nested 2-up input row.
    // gap-x for columns, gap-y for rows.
    <div className={cn("grid grid-cols-1 gap-x-4 gap-y-5 sm:grid-cols-2", className)}>
      {/* Ordering — empty string means "inherit the channel's Strategy". Radix Select
          forbids an empty-string item value, so "inherit" is the sentinel. */}
      {showOrdering && (
        <div className="flex flex-col gap-1.5">
          <FieldLabel
            htmlFor="policy-ordering"
            help="The channel's normal play order. Scheduled rules can temporarily override it."
          >
            Play order
          </FieldLabel>
          <Select
            value={policy.ordering || "inherit"}
            onValueChange={(v) => onChange({ ...policy, ordering: v === "inherit" ? "" : v })}
          >
            <SelectTrigger id="policy-ordering">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {orderingOptions(strategy).map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}

      {/* Audience ceiling — a safety limit, never relaxed by the ladder (§8), so the
          help says so explicitly rather than leaving that guarantee implicit. */}
      {showScope && (
        <div className="flex flex-col gap-1.5">
          <FieldLabel
            htmlFor="policy-ceiling"
            help="Content stays at or below this, a safety limit Loomarr never loosens."
          >
            Audience ceiling
          </FieldLabel>
          <Select
            value={policy.audience?.ceiling || "none"}
            onValueChange={(v) =>
              onChange({
                ...policy,
                audience: { ...policy.audience, ceiling: v === "none" ? "" : v },
              })
            }
          >
            <SelectTrigger id="policy-ceiling">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {CEILING_OPTIONS.map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}

      {/* Unrated titles — the safety PAIR to the ceiling, and orphaned until now: the gate has
          always been enforced (and reported in the exclusion report) while nothing could choose
          it, so an operator could see "3 skipped: unrated" and had no way to say "allow them".
          Sits beside the ceiling because its default is DERIVED from it. */}
      {showScope && (
        <div className="flex flex-col gap-1.5">
          <FieldLabel
            htmlFor="policy-unrated"
            help="Titles with no content rating. Automatic follows the ceiling: strict for a kids ceiling, permissive otherwise."
          >
            Unrated titles
          </FieldLabel>
          <Select
            value={policy.audience?.unrated || "default"}
            onValueChange={(v) =>
              onChange({
                ...policy,
                audience: { ...policy.audience, unrated: v === "default" ? "" : v },
              })
            }
          >
            <SelectTrigger id="policy-unrated">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {UNRATED_OPTIONS.map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {/* "Automatic" alone is not actionable — the operator cannot tell which way it
                      falls without knowing the kids-ceiling rule. Naming the current resolution
                      turns it into a real choice. */}
                  {o.value === "default"
                    ? `${o.label}: ${unratedResolvesToExclude(policy.audience?.ceiling) ? "skipped" : "allowed"}`
                    : o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}

      {/* Era — two commit-on-blur year inputs. Blank on either side means unbounded, not 0.
          Spans both columns (it has its own nested 2-up row). */}
      {showScope && (
        <div className="flex flex-col gap-1.5 sm:col-span-2">
          <FieldLabel help="Restrict content to titles released in this range. Leave blank for no restriction.">
            Era
          </FieldLabel>
          <div className="flex items-center gap-3">
            <div className="flex flex-col gap-1">
              <Label htmlFor="policy-era-from" className="text-muted-foreground text-xs">
                From year
              </Label>
              <Input
                id="policy-era-from"
                type="number"
                className="w-28"
                defaultValue={era?.from ?? ""}
                placeholder="Any"
                onBlur={(e) => {
                  const next = e.target.value === "" ? undefined : Number(e.target.value);
                  if (next === era?.from) return;
                  const { dates: _dates, ...scope } = policy.scope ?? {};
                  onChange({
                    ...policy,
                    scope: { ...scope, era: { ...era, from: next } },
                  });
                }}
              />
            </div>
            <div className="flex flex-col gap-1">
              <Label htmlFor="policy-era-to" className="text-muted-foreground text-xs">
                To year
              </Label>
              <Input
                id="policy-era-to"
                type="number"
                className="w-28"
                defaultValue={era?.to ?? ""}
                placeholder="Any"
                onBlur={(e) => {
                  const next = e.target.value === "" ? undefined : Number(e.target.value);
                  if (next === era?.to) return;
                  const { dates: _dates, ...scope } = policy.scope ?? {};
                  onChange({
                    ...policy,
                    scope: { ...scope, era: { ...era, to: next } },
                  });
                }}
              />
            </div>
          </div>
        </div>
      )}

      {showScope && (
        <div className="flex flex-col gap-2 sm:col-span-2">
          <FieldLabel help="Use separate year ranges when the channel needs distinct release, series-premiere, or episode-airing dates. Ranges are inclusive and never fill a gap between them.">
            Programming dates
          </FieldLabel>
          <p className="text-muted-foreground text-xs">
            Adding date ranges replaces the single Era field. Movie release and series premiere filter titles;
            episode airing filters individual episodes.
          </p>
          {(
            [
              ["Movie release", "movieRelease"],
              ["Series premiere", "seriesPremiere"],
              ["Episode airing", "seriesAiring"],
            ] as const
          ).map(([label, axis]) => (
            <DateAxisEditor
              key={axis}
              label={label}
              ranges={normalizedRanges(dates?.[axis])}
              onChange={(next) => {
                const nextAxis = normalizedRanges(next);
                const nextDates = { ...dates, [axis]: nextAxis.length ? nextAxis : undefined };
                const hasDates = Object.values(nextDates).some(
                  (ranges) => Array.isArray(ranges) && ranges.length > 0,
                );
                const { era: _era, dates: _dates, ...scope } = policy.scope ?? {};
                onChange({
                  ...policy,
                  scope: { ...scope, ...(hasDates ? { dates: nextDates } : {}) },
                });
              }}
            />
          ))}
        </div>
      )}

      {/* Longest programme — orphaned until now: the backend has filtered on runtimeMax
          since the scope policy existed, and nothing could set it. Useful for a channel
          that should stay to half-hour episodes and never pull in a three-hour film. */}
      {showScope && (
        <div className="flex flex-col gap-1.5">
          <FieldLabel
            htmlFor="policy-runtime-max"
            help="Skip anything longer than this. Leave blank for no limit."
          >
            Longest programme
          </FieldLabel>
          {/* MINUTES in the field, SECONDS on the wire (schedule.ScopePolicy.RuntimeMax is
              seconds, 0 = unbounded). Nobody thinks about programme length in seconds, and
              a raw seconds box would invite "90" meaning a minute and a half. */}
          <Input
            id="policy-runtime-max"
            className="w-28"
            type="number"
            min={0}
            defaultValue={policy.scope?.runtimeMax ? Math.round(policy.scope.runtimeMax / 60) : ""}
            placeholder="e.g. 90"
            onBlur={(e) => {
              const raw = e.target.value.trim();
              // Blank and 0 both mean unbounded, and the wire spells that 0 — so a cleared
              // field must send 0, not undefined, or the omitempty drops it and the old
              // value survives the merge.
              const next = raw === "" ? 0 : Math.max(0, Math.round(Number(raw) * 60));
              if (next === (policy.scope?.runtimeMax ?? 0)) return;
              onChange({ ...policy, scope: { ...policy.scope, runtimeMax: next } });
            }}
          />
        </div>
      )}

      {/* No-repeat windows — commit-on-blur duration strings. Spans both columns (nested
          2-up row). */}
      {showOrdering && (
        <div className="flex flex-col gap-1.5 sm:col-span-2">
          <FieldLabel help="Minimum time before content repeats. Leave blank to use Loomarr's built-in spacing.">
            Repeat spacing
          </FieldLabel>
          <div className="flex items-center gap-3">
            <div className="flex flex-col gap-1">
              <Label htmlFor="policy-movie-norepeat" className="text-muted-foreground text-xs">
                Same movie
              </Label>
              <Input
                id="policy-movie-norepeat"
                className="w-28"
                defaultValue={tidyDuration(separation?.movieNoRepeat)}
                placeholder="Use built-in"
                onBlur={(e) => {
                  const next = durationOrUndefined(e.target.value);
                  if (next === tidyDuration(separation?.movieNoRepeat) || next === separation?.movieNoRepeat)
                    return;
                  onChange({ ...policy, separation: { ...separation, movieNoRepeat: next } });
                }}
              />
            </div>
            <div className="flex flex-col gap-1">
              <Label htmlFor="policy-episode-norepeat" className="text-muted-foreground text-xs">
                Same episode
              </Label>
              <Input
                id="policy-episode-norepeat"
                className="w-28"
                defaultValue={tidyDuration(separation?.episodeNoRepeat)}
                placeholder="Use built-in"
                onBlur={(e) => {
                  const next = durationOrUndefined(e.target.value);
                  if (
                    next === tidyDuration(separation?.episodeNoRepeat) ||
                    next === separation?.episodeNoRepeat
                  )
                    return;
                  onChange({ ...policy, separation: { ...separation, episodeNoRepeat: next } });
                }}
              />
            </div>
          </div>

          {/* Series gap + block cap — the OTHER half of separation (§3). These were
              orphaned: the relaxation ladder narrates them in Diagnostics ("series gap
              was 2h") while nothing could set them, so an operator could read what the
              scheduler had eased and had no way to choose it in the first place. */}
          <div className="mt-3 flex flex-wrap gap-3">
            <div className="flex flex-col gap-1">
              <Label htmlFor="policy-series-gap" className="text-muted-foreground text-xs">
                Same series
              </Label>
              <Input
                id="policy-series-gap"
                className="w-28"
                defaultValue={tidyDuration(separation?.seriesMinGap)}
                placeholder="Use built-in"
                onBlur={(e) => {
                  const next = durationOrUndefined(e.target.value);
                  if (next === tidyDuration(separation?.seriesMinGap) || next === separation?.seriesMinGap)
                    return;
                  onChange({ ...policy, separation: { ...separation, seriesMinGap: next } });
                }}
              />
            </div>
            <div className="flex flex-col gap-1">
              <Label htmlFor="policy-block-max" className="text-muted-foreground text-xs">
                Max from one series
              </Label>
              <Input
                id="policy-block-max"
                className="w-28"
                type="number"
                min={0}
                defaultValue={separation?.blockMax ?? ""}
                placeholder="Use built-in"
                onBlur={(e) => {
                  // Blank clears the authored cap and returns to the built-in limit. `0` is
                  // also resolved as built-in here, so there is no separate no-limit state.
                  const raw = e.target.value.trim();
                  const next = raw === "" ? undefined : Number(raw);
                  if (next === separation?.blockMax) return;
                  onChange({ ...policy, separation: { ...separation, blockMax: next } });
                }}
              />
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export { ChannelPolicyFields };
