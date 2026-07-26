import { Input, Label, Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui";
import { cn } from "@/lib";
import { FieldHelp } from "../../feedback";
import type { ChannelPolicyFieldsProps } from "./channel-policy-fields.type";

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
// clears the window entirely (undefined — "no restriction"), not zero.

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
// or undefined for blank — undefined = "no restriction", never "0" (which would mean the
// real, different thing "never repeat"). Whitespace-only is treated as blank.
const durationOrUndefined = (v: string): string | undefined => {
  const trimmed = v.trim();
  return trimmed === "" ? undefined : trimmed;
};

const ORDERING_OPTIONS: { value: string; label: string }[] = [
  { value: "inherit", label: "Inherit channel default" },
  { value: "sequential", label: "In order" },
  { value: "shuffle", label: "Shuffled" },
  { value: "syndication", label: "Syndication" },
];

// The CHANNEL's playback strategy (ChannelDTO.strategy), the value "Inherit channel default"
// above refers to. Note it is NOT the same set as ORDERING_OPTIONS: `time_slot` is a channel
// strategy with no policy-level equivalent, and `syndication` is the reverse.
const STRATEGY_OPTIONS: { value: string; label: string }[] = [
  { value: "sequential", label: "In order" },
  { value: "shuffle", label: "Shuffled" },
  { value: "time_slot", label: "Time slots" },
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
  onStrategyChange,
}: ChannelPolicyFieldsProps) => {
  const era = policy.scope?.era;
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
      {/* Channel strategy — the value Ordering's "inherit" refers to. Orphaned until now:
          PATCH accepted `strategy` and no UI sent it, so "Inherit channel default" pointed
          at something an operator could neither see nor choose. Rendered only when the
          caller supplies it (the standalone policy form has no channel in scope). */}
      {showOrdering && strategy !== undefined && onStrategyChange && (
        <div className="flex flex-col gap-1.5">
          <FieldLabel
            htmlFor="channel-strategy"
            help="The channel's default playback order. Ordering below can override it per policy."
          >
            Playback
          </FieldLabel>
          <Select value={strategy || "sequential"} onValueChange={onStrategyChange}>
            <SelectTrigger id="channel-strategy">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {STRATEGY_OPTIONS.map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}

      {/* Ordering — empty string means "inherit the channel's Strategy". Radix Select
          forbids an empty-string item value, so "inherit" is the sentinel. */}
      {showOrdering && (
        <div className="flex flex-col gap-1.5">
          <FieldLabel htmlFor="policy-ordering" help="How programs are sequenced on this channel.">
            Ordering
          </FieldLabel>
          <Select
            value={policy.ordering || "inherit"}
            onValueChange={(v) => onChange({ ...policy, ordering: v === "inherit" ? "" : v })}
          >
            <SelectTrigger id="policy-ordering">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {ORDERING_OPTIONS.map((o) => (
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
            help="Content stays at or below this — a safety limit Loomarr never loosens."
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
                  onChange({
                    ...policy,
                    scope: { ...policy.scope, era: { ...era, from: next } },
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
                  onChange({
                    ...policy,
                    scope: { ...policy.scope, era: { ...era, to: next } },
                  });
                }}
              />
            </div>
          </div>
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
          <FieldLabel help="How long before the same title can play again (e.g. 168h = 7 days).">
            No-repeat windows
          </FieldLabel>
          <div className="flex items-center gap-3">
            <div className="flex flex-col gap-1">
              <Label htmlFor="policy-movie-norepeat" className="text-muted-foreground text-xs">
                Movies
              </Label>
              <Input
                id="policy-movie-norepeat"
                className="w-28"
                defaultValue={tidyDuration(separation?.movieNoRepeat)}
                placeholder="e.g. 168h"
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
                Episodes
              </Label>
              <Input
                id="policy-episode-norepeat"
                className="w-28"
                defaultValue={tidyDuration(separation?.episodeNoRepeat)}
                placeholder="e.g. 24h"
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
                Series gap
              </Label>
              <Input
                id="policy-series-gap"
                className="w-28"
                defaultValue={tidyDuration(separation?.seriesMinGap)}
                placeholder="e.g. 2h"
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
                Max in a row
              </Label>
              <Input
                id="policy-block-max"
                className="w-28"
                type="number"
                min={0}
                defaultValue={separation?.blockMax ?? ""}
                placeholder="e.g. 2"
                onBlur={(e) => {
                  // Blank clears the cap (undefined = "no limit"), never 0 — which would
                  // mean the different and impossible "never air two in a row".
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
