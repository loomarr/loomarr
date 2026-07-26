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

// Both enums use the `"" = default` sentinel. Radix Select forbids an empty-string item value,
// so each list carries a named sentinel that lowers back to "" on the wire — the same shape as
// Ordering's "inherit" and the unrated "Automatic".
const MODE_OPTIONS: { value: string; label: string }[] = [
  { value: "default", label: "Automatic" },
  { value: "off", label: "Ignore holidays" },
  { value: "auto", label: "Lean into the season" },
  { value: "exclusive", label: "Holiday channel" },
];

const OFF_SEASON_OPTIONS: { value: string; label: string }[] = [
  { value: "default", label: "Play everything (default)" },
  { value: "loop", label: "Play everything" },
  { value: "dark", label: "Go dark" },
];

// What each mode does, in the operator's words. Shown under the select because "exclusive"
// vs "auto" is not self-evident and the difference is what the channel plays in December.
const MODE_HINT: Record<string, string> = {
  default: "Same as leaning into the season.",
  off: "Holidays are ignored — nothing is boosted or benched.",
  auto: "In-season titles come up more often; out-of-season ones are held back.",
  exclusive: "The channel IS the holiday — only in-season titles play during the window.",
};

// The holiday tokens the BE publishes, as `holiday:<id>` entries in the rule vocabulary's
// `when` list. Stripping the prefix yields the id `policy.seasonal.holidays` stores.
const HOLIDAY_PREFIX = "holiday:";

const ChannelSeasonal = ({ policy, onChange, vocabulary, className }: ChannelSeasonalProps) => {
  const seasonal = policy.seasonal;
  const mode = seasonal?.mode || "default";
  const selected = seasonal?.holidays ?? [];

  const holidays = (vocabulary.when ?? [])
    .filter((w) => w.token.startsWith(HOLIDAY_PREFIX))
    .map((w) => ({ id: w.token.slice(HOLIDAY_PREFIX.length), label: w.label }));

  const toggleHoliday = (id: string, on: boolean) => {
    // Keep the stored order stable (vocabulary order), so a save does not reshuffle the list
    // and make an unrelated diff look like a change.
    const next = on
      ? holidays.filter((h) => h.id === id || selected.includes(h.id)).map((h) => h.id)
      : selected.filter((h) => h !== id);
    onChange({
      ...policy,
      seasonal: { ...seasonal, holidays: next },
    });
  };

  return (
    <div className={cn("flex flex-col gap-4", className)}>
      <div className="flex flex-col gap-1.5">
        <div className="flex items-center gap-1.5">
          <Label htmlFor="policy-seasonal-mode">Holidays</Label>
          <FieldHelp label="Holidays">
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
          <legend className="pb-1 text-muted-foreground text-xs">
            Which holidays {selected.length === 0 ? "(none picked — all of them)" : ""}
          </legend>
          <div className="flex flex-wrap gap-x-5 gap-y-2">
            {holidays.map((h) => (
              <div key={h.id} className="flex items-center gap-2">
                <Checkbox
                  id={`policy-holiday-${h.id}`}
                  checked={selected.includes(h.id)}
                  onChange={(e) => toggleHoliday(h.id, e.target.checked)}
                />
                <Label htmlFor={`policy-holiday-${h.id}`} className="font-normal">
                  {h.label}
                </Label>
              </div>
            ))}
          </div>
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
            value={seasonal?.offSeason || "default"}
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
