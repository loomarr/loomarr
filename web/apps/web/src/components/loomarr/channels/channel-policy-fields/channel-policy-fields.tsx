import { tidyGoDuration } from "@loomarr/core/format";
import { Action, CompactSelect, Stack, Text } from "@loomarr/design-system";
import { CommitField, normalizeYearRanges, YearRangeListField } from "@loomarr/ui";
import { useState } from "react";
import { eraDates, eraOf } from "@/lib/era-dates";
import { describePlayOrder, INHERIT_PLAY_ORDER_LABEL, PLAY_ORDER_LABEL } from "@/lib/play-order-copy";
import {
  datesMode,
  isOverridden,
  type PolicyField as PolicyFieldName,
  resetField,
} from "@/lib/policy-overrides";
import { cn } from "@/lib/utils";
import { FieldHelp } from "../../feedback";
import { PolicyFieldSource } from "../policy-field-source";
import type { ChannelPolicyFieldsProps } from "./channel-policy-fields.type";

// One date axis: its title over the shared range list.
const DateAxisEditor = ({
  label,
  ranges,
  onChange,
}: {
  label: string;
  ranges: { from: number; to: number }[];
  onChange: (next: { from: number; to: number }[]) => void;
}) => (
  <Stack className="border-border rounded-md border p-3" gap="inline">
    <Stack align="center" direction="row" gap="control" justify="between">
      <Text textRole="label">{label}</Text>
    </Stack>
    <YearRangeListField emptyText="No date requirement." label={label} onChange={onChange} ranges={ranges} />
  </Stack>
);

// PolicyField — one defaultable field (#1817 item 4): its label and (i) help on the left, where
// the value comes from and a Reset on the right, the control below. The (i) icon replaces the
// old permanent helper `<p>` under every control, so the guidance is on hover, not always on
// screen. `htmlFor` ties the label to a single control; a field with several inputs leaves it
// out and labels its group by `labelId` instead.
const PolicyField = ({
  id,
  htmlFor,
  label,
  help,
  overridden,
  onReset,
  wide,
  children,
}: {
  id: string;
  htmlFor?: string;
  label: string;
  help: string;
  overridden: boolean;
  onReset: () => void;
  wide?: boolean;
  children: React.ReactNode;
}) => {
  const labelId = `${id}-label`;
  return (
    <div className={cn("flex flex-col gap-1.5", wide && "sm:col-span-2")}>
      <Stack align="center" direction="row" gap="inline" justify="between" wrap>
        <Stack align="center" direction="row" gap="1.5">
          <Text as="label" htmlFor={htmlFor} id={labelId} textRole="label">
            {label}
          </Text>
          <FieldHelp label={label}>{help}</FieldHelp>
        </Stack>
        <PolicyFieldSource overridden={overridden} onReset={onReset} label={label} />
      </Stack>
      {children}
    </div>
  );
};

// ChannelPolicyFields — a ChannelPolicy as plain-language editable chips (programming-design
// §8). Controlled: the parent holds `policy` and persists whatever this calls `onChange`
// with. `applied` is reconcile-owned (§9) and never rendered here — it is read-only history
// of what the scheduler had to ease, not a setting an operator edits. A field left blank
// clears the authored override; its effective behavior is field-specific and named below.

// On save: pass the typed duration straight through (the backend accepts "24h"/"168h"),
// or undefined for blank — undefined returns to Loomarr's built-in spacing. Whitespace-only
// is treated as blank.
const durationOrUndefined = (v: string): string | undefined => {
  const trimmed = v.trim();
  return trimmed === "" ? undefined : trimmed;
};

// The three separation windows, each a Go-duration string whose blank means built-in spacing.
const DURATION_FIELDS = [
  {
    field: "movieNoRepeat",
    id: "policy-movie-norepeat",
    label: "Repeat spacing · same movie",
    help: "Minimum time before a movie repeats. Leave blank to use Loomarr's built-in spacing.",
  },
  {
    field: "episodeNoRepeat",
    id: "policy-episode-norepeat",
    label: "Repeat spacing · same episode",
    help: "Minimum time before an episode repeats. Leave blank to use Loomarr's built-in spacing.",
  },
  {
    field: "seriesMinGap",
    id: "policy-series-gap",
    label: "Repeat spacing · same series",
    help: "Minimum gap for another episode of the same series. Leave blank to use Loomarr's built-in spacing.",
  },
] as const satisfies readonly { field: PolicyFieldName; id: string; label: string; help: string }[];

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
      ? `${INHERIT_PLAY_ORDER_LABEL} (${STRATEGY_LABELS[strategy] ?? strategy})`
      : INHERIT_PLAY_ORDER_LABEL,
  },
  ...Object.entries(PLAY_ORDER_LABEL).map(([value, label]) => ({ value, label })),
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

const ChannelPolicyFields = ({
  policy,
  onChange,
  className,
  show,
  strategy,
  cycle,
}: ChannelPolicyFieldsProps) => {
  // The mode that will actually run, for the consequence line: the policy's own choice, else
  // what the scheduler resolved for "use channel strategy" (cycle.ordering), else the strategy.
  const effectiveOrdering = policy.ordering || cycle?.ordering || strategy || "";
  const orderConsequence = describePlayOrder(effectiveOrdering, cycle);
  // The era is read back from era-shaped dates: the backend stores and returns only `dates`.
  const dates = policy.scope?.dates;
  const era = eraOf(dates);
  const separation = policy.separation;
  // Split for the Programming surface's blocks (§12): scope = audience ceiling + dates ("What
  // plays"); ordering = ordering + spacing ("How it's ordered"). Omitted = show everything.
  const showScope = show !== "ordering";
  const showOrdering = show !== "scope";

  // Era is the simple mode of the dates editor (#1877 decision 2): shown whenever the dates are
  // era-shaped or empty. Anything an era cannot express needs the per-axis editor. The operator
  // can also open the per-axis editor on purpose; a reset to default returns to Era.
  const [axesRequested, setAxesRequested] = useState(false);
  const eraShaped = datesMode(dates) === "era";
  const showAxes = !eraShaped || axesRequested;

  // Every field reads its source and resets through the same sentinel table the summary counts.
  const source = (field: PolicyFieldName) => ({
    overridden: isOverridden(policy, field),
    onReset: () => onChange(resetField(policy, field)),
  });

  return (
    // A responsive 2-column field grid (was a 1-wide stack): the Selects and single inputs sit
    // side by side, while the dates editor spans the full width. gap-x for columns, gap-y for rows.
    <div className={cn("grid grid-cols-1 gap-x-4 gap-y-5 sm:grid-cols-2", className)}>
      {/* Ordering — empty string means "inherit the channel's Strategy"; the select names that
          choice "inherit". */}
      {showOrdering && (
        <PolicyField
          id="policy-ordering"
          htmlFor="policy-ordering"
          label="Play order"
          help="The channel's normal play order. In order plays the list start to finish and starts over. Shuffled plays a random mix. Syndication deals the list like a deck of cards, so everything plays once before anything repeats. Scheduled rules can temporarily override it."
          {...source("ordering")}
        >
          <CompactSelect
            aria-describedby={orderConsequence ? "policy-ordering-consequence" : undefined}
            id="policy-ordering"
            label="Play order"
            onValueChange={(v) => onChange({ ...policy, ordering: v === "inherit" ? "" : v })}
            options={orderingOptions(strategy)}
            value={policy.ordering || "inherit"}
          />
          {orderConsequence && (
            <Text as="p" id="policy-ordering-consequence" textRole="caption" tone="muted">
              {orderConsequence}
            </Text>
          )}
        </PolicyField>
      )}

      {/* Audience ceiling — a safety limit, never relaxed by the ladder (§8), so the
          help says so explicitly rather than leaving that guarantee implicit. */}
      {showScope && (
        <PolicyField
          id="policy-ceiling"
          htmlFor="policy-ceiling"
          label="Audience ceiling"
          help="Content stays at or below this, a safety limit Loomarr never loosens."
          {...source("ceiling")}
        >
          <CompactSelect
            id="policy-ceiling"
            label="Audience ceiling"
            onValueChange={(v) =>
              onChange({
                ...policy,
                audience: { ...policy.audience, ceiling: v === "none" ? "" : v },
              })
            }
            options={CEILING_OPTIONS}
            value={policy.audience?.ceiling || "none"}
          />
        </PolicyField>
      )}

      {/* Unrated titles — the safety PAIR to the ceiling, and orphaned until now: the gate has
          always been enforced (and reported in the exclusion report) while nothing could choose
          it, so an operator could see "3 skipped: unrated" and had no way to say "allow them".
          Sits beside the ceiling because its default is DERIVED from it. */}
      {showScope && (
        <PolicyField
          id="policy-unrated"
          htmlFor="policy-unrated"
          label="Unrated titles"
          help="Titles with no content rating. Automatic follows the ceiling: strict for a kids ceiling, permissive otherwise."
          {...source("unrated")}
        >
          <CompactSelect
            id="policy-unrated"
            label="Unrated titles"
            onValueChange={(v) =>
              onChange({
                ...policy,
                audience: { ...policy.audience, unrated: v === "default" ? "" : v },
              })
            }
            // "Automatic" alone is not actionable — the operator cannot tell which way it falls
            // without knowing the kids-ceiling rule. Naming the current resolution turns it into a
            // real choice.
            options={UNRATED_OPTIONS.map((o) =>
              o.value === "default"
                ? {
                    ...o,
                    label: `${o.label}: ${unratedResolvesToExclude(policy.audience?.ceiling) ? "skipped" : "allowed"}`,
                  }
                : o,
            )}
            value={policy.audience?.unrated || "default"}
          />
        </PolicyField>
      )}

      {/* Dates — ONE stored scope (#1883). Era mode is two commit-on-blur year inputs (blank on
          either side means unbounded, not 0) written to all three axes; the per-axis editor
          holds anything an era cannot express. Spans both columns. */}
      {showScope && !showAxes && (
        <PolicyField
          id="policy-dates"
          label="Era"
          help="Restrict content to titles released in this range. Leave blank for no restriction."
          wide
          {...source("dates")}
        >
          <Stack
            align="end"
            aria-labelledby="policy-dates-label"
            direction="row"
            gap="control"
            role="group"
            wrap
          >
            {/* Keyed on the stored value: a reset or any outside change re-seeds the box instead
                of leaving the old year on screen. */}
            <CommitField
              id="policy-era-from"
              initial={String(era?.from ?? "")}
              key={`from-${era?.from ?? ""}`}
              keyboardType="number-pad"
              label="From year"
              maxLength={4}
              onCommit={(text) => {
                const next = text === "" ? undefined : Number(text);
                if (next === era?.from) return;
                const { dates: _dates, ...scope } = policy.scope ?? {};
                const nextDates = eraDates({ ...era, from: next });
                onChange({
                  ...policy,
                  scope: { ...scope, ...(nextDates ? { dates: nextDates } : {}) },
                });
              }}
              placeholder="Any"
              width={112}
            />
            <CommitField
              id="policy-era-to"
              initial={String(era?.to ?? "")}
              key={`to-${era?.to ?? ""}`}
              keyboardType="number-pad"
              label="To year"
              maxLength={4}
              onCommit={(text) => {
                const next = text === "" ? undefined : Number(text);
                if (next === era?.to) return;
                const { dates: _dates, ...scope } = policy.scope ?? {};
                const nextDates = eraDates({ ...era, to: next });
                onChange({
                  ...policy,
                  scope: { ...scope, ...(nextDates ? { dates: nextDates } : {}) },
                });
              }}
              placeholder="Any"
              width={112}
            />
            {/* Lifted 10 px so the line sits level with the year boxes' text, not their bottom edge. */}
            <Action
              layout="text"
              onPress={() => setAxesRequested(true)}
              style={{ marginBottom: 10 }}
              variant="ghost"
            >
              Use separate date windows
            </Action>
          </Stack>
        </PolicyField>
      )}

      {showScope && showAxes && (
        <PolicyField
          id="policy-dates"
          label="Programming dates"
          help="Separate movie-release, series-premiere and episode-airing windows. Ranges are inclusive and never fill a gap between them."
          wide
          overridden={isOverridden(policy, "dates")}
          onReset={() => {
            setAxesRequested(false);
            onChange(resetField(policy, "dates"));
          }}
        >
          <p className="text-muted-foreground text-xs">
            Movie release and series premiere filter titles; episode airing filters individual episodes.
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
              ranges={normalizeYearRanges(dates?.[axis])}
              onChange={(nextAxis) => {
                const nextDates = { ...dates, [axis]: nextAxis.length ? nextAxis : undefined };
                const hasDates = Object.values(nextDates).some(
                  (ranges) => Array.isArray(ranges) && ranges.length > 0,
                );
                const { era: _era, dates: _dates, ...scope } = policy.scope ?? {};
                // Editing an axis keeps this editor open even if the windows happen to line up
                // again, so the field does not swap under the operator mid-edit.
                setAxesRequested(true);
                onChange({
                  ...policy,
                  scope: { ...scope, ...(hasDates ? { dates: nextDates } : {}) },
                });
              }}
            />
          ))}
          {/* Back to Era only while that loses nothing: the windows are identical or empty. */}
          {eraShaped && (
            <Action
              layout="text"
              onPress={() => setAxesRequested(false)}
              style={{ alignSelf: "flex-start" }}
              variant="ghost"
            >
              Use a single era
            </Action>
          )}
        </PolicyField>
      )}

      {/* Longest programme — orphaned until now: the backend has filtered on runtimeMax
          since the scope policy existed, and nothing could set it. Useful for a channel
          that should stay to half-hour episodes and never pull in a three-hour film. */}
      {showScope && (
        <PolicyField
          id="policy-runtime-max"
          htmlFor="policy-runtime-max"
          label="Longest programme"
          help="Skip anything longer than this. Leave blank for no limit."
          {...source("runtimeMax")}
        >
          {/* MINUTES in the field, SECONDS on the wire (schedule.ScopePolicy.RuntimeMax is
              seconds, 0 = unbounded). Nobody thinks about programme length in seconds, and
              a raw seconds box would invite "90" meaning a minute and a half. */}
          <CommitField
            id="policy-runtime-max"
            initial={policy.scope?.runtimeMax ? String(Math.round(policy.scope.runtimeMax / 60)) : ""}
            key={policy.scope?.runtimeMax ?? 0}
            keyboardType="number-pad"
            placeholder="e.g. 90"
            width={112}
            onCommit={(text) => {
              const raw = text.trim();
              // Blank and 0 both mean unbounded, and the wire spells that 0 — so a cleared
              // field must send 0, not undefined, or the omitempty drops it and the old
              // value survives the merge.
              const next = raw === "" ? 0 : Math.max(0, Math.round(Number(raw) * 60));
              if (next === (policy.scope?.runtimeMax ?? 0)) return;
              onChange({ ...policy, scope: { ...policy.scope, runtimeMax: next } });
            }}
          />
        </PolicyField>
      )}

      {/* Repeat spacing — commit-on-blur duration strings, one field per window so each shows
          its own source and Reset. Series gap + block cap are the OTHER half of separation
          (§3). They were orphaned: the relaxation ladder narrates them in Diagnostics ("series
          gap was 2h") while nothing could set them. */}
      {showOrdering &&
        DURATION_FIELDS.map(({ field, id, label, help }) => (
          <PolicyField key={field} id={id} htmlFor={id} label={label} help={help} {...source(field)}>
            <CommitField
              autoCapitalize="none"
              id={id}
              initial={tidyGoDuration(separation?.[field])}
              key={separation?.[field] ?? ""}
              placeholder="Use built-in"
              width={160}
              onCommit={(text) => {
                const next = durationOrUndefined(text);
                if (next === tidyGoDuration(separation?.[field]) || next === separation?.[field]) return;
                onChange({ ...policy, separation: { ...separation, [field]: next } });
              }}
            />
          </PolicyField>
        ))}

      {showOrdering && (
        <PolicyField
          id="policy-block-max"
          htmlFor="policy-block-max"
          label="Max from one series"
          help="How many in a row the same series may hold. Leave blank to use Loomarr's built-in limit."
          {...source("blockMax")}
        >
          <CommitField
            id="policy-block-max"
            initial={String(separation?.blockMax ?? "")}
            key={separation?.blockMax ?? ""}
            keyboardType="number-pad"
            placeholder="Use built-in"
            width={160}
            onCommit={(text) => {
              // Blank clears the authored cap and returns to the built-in limit. `0` is
              // also resolved as built-in here, so there is no separate no-limit state.
              const raw = text.trim();
              const next = raw === "" ? undefined : Number(raw);
              if (next === separation?.blockMax) return;
              onChange({ ...policy, separation: { ...separation, blockMax: next } });
            }}
          />
        </PolicyField>
      )}
    </div>
  );
};

export { ChannelPolicyFields };
