import type { HolidayEpisodesDTO } from "@loomarr/api/models/holidayEpisodesDTO";
import { ChannelSeasonalFields, type SeasonalMode, type SeasonalStrength } from "@loomarr/ui";
import { isOverridden, resetField } from "@/lib/policy-overrides";
import { PolicyFieldSource } from "../policy-field-source";
import type { ChannelSeasonalProps } from "./channel-seasonal.type";

// ChannelSeasonal — holiday-aware programming (programming-design.md §6, schedule.SeasonalPolicy).
//
// The screen is `ChannelSeasonalFields` in @loomarr/ui; this file only maps the channel policy onto
// it. The scheduler has always benched out-of-season titles and boosted in-season ones; the
// operator now chooses it. `builtinCalendar` (schedule/seasonal.go) is a FIXED list whose ids
// already reach the frontend through the rule vocabulary, so the holiday picker is a checkbox
// list over a closed set.

// The holiday tokens the BE publishes, as `holiday:<id>` entries in the rule vocabulary's
// `when` list. Stripping the prefix yields the id `policy.seasonal.holidays` stores.
const HOLIDAY_PREFIX = "holiday:";

// The backend's count for the holiday that is on now, or the next one (it sends them soonest
// first). Plain words: what the chosen strength will actually do with this lineup.
const consequence = (h: HolidayEpisodesDTO, label: string): string => {
  const when = h.active ? `${label} is on now` : `Next up: ${label}`;
  if (h.episodes === 0) return `${when}. No ${label} episodes found in this lineup.`;
  const found = `${h.episodes} ${label} ${h.episodes === 1 ? "episode" : "episodes"} found`;
  const rate =
    h.perDay === 1 && h.everyDays > 1 && h.episodes === 1
      ? `it airs every ${h.everyDays} days`
      : `${h.perDay} a day, each back about every ${h.everyDays} days`;
  return `${when}. ${found}: ${rate}.`;
};

// Every enum collapses the `"" = default` sentinel and its explicit twin (`auto`, `featured`,
// `loop`) into one choice; writing `default` stores the sentinel.
const stored = <T extends string>(value: T) => (value === "default" ? "" : value);

const ChannelSeasonal = ({ policy, onChange, vocabulary, holidayEpisodes }: ChannelSeasonalProps) => {
  const seasonal = policy.seasonal;
  const mode: SeasonalMode =
    !seasonal?.mode || seasonal.mode === "auto" ? "default" : (seasonal.mode as SeasonalMode);
  const selected = seasonal?.holidays ?? [];
  const strength: SeasonalStrength =
    !seasonal?.strength || seasonal.strength === "featured"
      ? "default"
      : (seasonal.strength as SeasonalStrength);

  const holidays = (vocabulary.when ?? [])
    .filter((w) => w.token.startsWith(HOLIDAY_PREFIX))
    .map((w) => ({ id: w.token.slice(HOLIDAY_PREFIX.length), label: w.label }));

  // Empty on the wire means every built-in holiday. Render the effective selection rather than
  // the serialization sentinel: an unchecked list that actually means “all” reverses the normal
  // checkbox contract. Once one is unchecked, materialize the remaining subset.
  const effectiveSelected = selected.length === 0 ? holidays.map((h) => h.id) : selected;

  const upcoming = holidayEpisodes?.[0];
  const upcomingLabel = upcoming && holidays.find((h) => h.id === upcoming.holiday)?.label;

  const setSeasonal = (patch: NonNullable<typeof seasonal>) =>
    onChange({ ...policy, seasonal: { ...seasonal, ...patch } });

  const toggleHoliday = (id: string, on: boolean) => {
    // Keep the stored order stable (vocabulary order), so a save does not reshuffle the list
    // and make an unrelated diff look like a change.
    const next = holidays
      .filter((h) => (h.id === id ? on : effectiveSelected.includes(h.id)))
      .map((h) => h.id);
    // Preserve the compact canonical wire form: all checked = empty subset = all built-ins.
    setSeasonal({ holidays: next.length === holidays.length ? [] : next });
  };

  return (
    <ChannelSeasonalFields
      allHolidaysIncluded={selected.length === 0}
      favourEvenings={seasonal?.placement !== "anytime"}
      holidays={holidays}
      includedHolidayIds={effectiveSelected}
      mode={mode}
      modeSource={
        // The mode is the defaultable field; the holiday subset and off-season fallback refine
        // it and keep what they hold.
        <PolicyFieldSource
          label="Seasonal behavior"
          onReset={() => onChange(resetField(policy, "seasonalMode"))}
          overridden={isOverridden(policy, "seasonalMode")}
        />
      }
      offSeason={!seasonal?.offSeason || seasonal.offSeason === "loop" ? "default" : "dark"}
      onFavourEveningsChange={(favour) => setSeasonal({ placement: favour ? "" : "anytime" })}
      onHolidayToggle={toggleHoliday}
      onModeChange={(next) => setSeasonal({ mode: stored(next) })}
      onOffSeasonChange={(next) => setSeasonal({ offSeason: stored(next) })}
      onStrengthChange={(next) => setSeasonal({ strength: stored(next) })}
      strength={strength}
      strengthConsequence={upcoming && upcomingLabel ? consequence(upcoming, upcomingLabel) : undefined}
    />
  );
};

export { ChannelSeasonal };
