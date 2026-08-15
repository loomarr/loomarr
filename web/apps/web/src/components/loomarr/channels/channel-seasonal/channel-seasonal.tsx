import {
  Checkbox,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui";
import { cn } from "@/lib";
import { FieldHelp } from "../../feedback";
import type { ChannelSeasonalProps } from "./channel-seasonal.type";

// ChannelSeasonal — holiday-aware programming (programming-design.md §6, schedule.SeasonalPolicy).
//
// ORPHANED until now: the scheduler has always benched out-of-season titles and boosted
// in-season ones, and the only way to set any of it was an LLM refine — a channel's operator
// could see the effect and never choose it. The audit filed this as "needs design work first"
// on the premise that the holiday picker required TMDB keyword lookups; reading the code, it
// does not. `builtinCalendar` (schedule/seasonal.go) is a FIXED list, and its ids already
// reach the frontend through the rule vocabulary, so this is a checkbox list over a closed set.

// Both enums use the `"" = default` sentinel. The explicit `auto` and `loop` wire values resolve
// identically to their defaults, so the interface deliberately collapses each pair into one
// choice. Storage provenance is not a user decision.
const MODE_OPTIONS: { value: string; label: string }[] = [
  { value: "default", label: "Use built-in seasonal rotation" },
  { value: "off", label: "Ignore holidays" },
  { value: "exclusive", label: "Holiday-only channel" },
];

const OFF_SEASON_OPTIONS: { value: string; label: string }[] = [
  { value: "default", label: "Keep regular programming" },
  { value: "dark", label: "Go dark" },
];

// What each mode does, in the operator's words. Shown under the select because "exclusive"
// vs "auto" is not self-evident and the difference is what the channel plays in December.
const MODE_HINT: Record<string, string> = {
  default:
    "Uses Loomarr's built-in holiday calendar. In-season titles play more often; other seasonal titles wait for their window.",
  off: "Holidays are ignored. Nothing is boosted or benched.",
  exclusive: "The channel IS the holiday: only in-season titles play during the window.",
};

// The holiday tokens the BE publishes, as `holiday:<id>` entries in the rule vocabulary's
// `when` list. Stripping the prefix yields the id `policy.seasonal.holidays` stores.
const HOLIDAY_PREFIX = "holiday:";

const ChannelSeasonal = ({ policy, onChange, vocabulary, className }: ChannelSeasonalProps) => {
  const seasonal = policy.seasonal;
  const mode = !seasonal?.mode || seasonal.mode === "auto" ? "default" : seasonal.mode;
  const selected = seasonal?.holidays ?? [];

  const holidays = (vocabulary.when ?? [])
    .filter((w) => w.token.startsWith(HOLIDAY_PREFIX))
    .map((w) => ({ id: w.token.slice(HOLIDAY_PREFIX.length), label: w.label }));

  // Empty on the wire means every built-in holiday. Render the effective selection rather than
  // the serialization sentinel: an unchecked list that actually means “all” reverses the normal
  // checkbox contract. Once one is unchecked, materialize the remaining subset.
  const effectiveSelected = selected.length === 0 ? holidays.map((h) => h.id) : selected;

  const toggleHoliday = (id: string, on: boolean) => {
    // Keep the stored order stable (vocabulary order), so a save does not reshuffle the list
    // and make an unrelated diff look like a change.
    const next = holidays
      .filter((h) => (h.id === id ? on : effectiveSelected.includes(h.id)))
      .map((h) => h.id);
    // Preserve the compact canonical wire form: all checked = empty subset = all built-ins.
    const stored = next.length === holidays.length ? [] : next;
    onChange({
      ...policy,
      seasonal: { ...seasonal, holidays: stored },
    });
  };

  return (
    <div className={cn("flex flex-col gap-4", className)}>
      <div className="flex flex-col gap-1.5">
        <div className="flex items-center gap-1.5">
          <Label htmlFor="policy-seasonal-mode">Seasonal behavior</Label>
          <FieldHelp label="Seasonal behavior">
            How this channel reacts to the calendar. Loomarr matches titles to a built-in holiday list; pick
            which holidays below.
          </FieldHelp>
        </div>
        <Select
          value={mode}
          onValueChange={(v) =>
            onChange({ ...policy, seasonal: { ...seasonal, mode: v === "default" ? "" : v } })
          }
        >
          <SelectTrigger id="policy-seasonal-mode" className="w-64">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {MODE_OPTIONS.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <p className="text-muted-foreground text-sm">{MODE_HINT[mode]}</p>
      </div>

      {/* Hidden when holidays are ignored: the list would set a value nothing reads. */}
      {mode !== "off" && (
        <fieldset className="flex flex-col gap-2">
          <legend className="pb-1 text-muted-foreground text-xs">Included holidays</legend>
          <div className="flex flex-wrap gap-x-5 gap-y-2">
            {holidays.map((h) => (
              <div key={h.id} className="flex items-center gap-2">
                <Checkbox
                  id={`policy-holiday-${h.id}`}
                  checked={effectiveSelected.includes(h.id)}
                  onChange={(e) => toggleHoliday(h.id, e.target.checked)}
                />
                <Label htmlFor={`policy-holiday-${h.id}`} className="font-normal">
                  {h.label}
                </Label>
              </div>
            ))}
          </div>
          {selected.length === 0 && (
            <p className="text-muted-foreground text-xs">All built-in holidays are included.</p>
          )}
        </fieldset>
      )}

      {/* Off-season fallback is read ONLY in exclusive mode (schedule/seasonal.go:154, inside
          `case SeasonalExclusive`), so it appears only there — offering it under `auto` would
          be a setting the engine never consults. */}
      {mode === "exclusive" && (
        <div className="flex flex-col gap-1.5">
          <div className="flex items-center gap-1.5">
            <Label htmlFor="policy-seasonal-offseason">Out of season</Label>
            <FieldHelp label="Out of season">What a holiday channel does the rest of the year.</FieldHelp>
          </div>
          <Select
            value={!seasonal?.offSeason || seasonal.offSeason === "loop" ? "default" : seasonal.offSeason}
            onValueChange={(v) =>
              onChange({
                ...policy,
                seasonal: { ...seasonal, offSeason: v === "default" ? "" : v },
              })
            }
          >
            <SelectTrigger id="policy-seasonal-offseason" className="w-64">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {OFF_SEASON_OPTIONS.map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}
    </div>
  );
};

export { ChannelSeasonal };
