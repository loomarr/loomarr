import type { ChoiceOption } from "@loomarr/design-system";
import { ChoiceGroup, FieldHint, Surface, Text, Toggle } from "@loomarr/design-system";

import type {
  ChannelSeasonalFieldsProps,
  SeasonalMode,
  SeasonalOffSeason,
  SeasonalStrength,
} from "./channel-seasonal-fields.type";

// What each mode does, in the operator's words. Shown with the choice because "exclusive" vs
// "auto" is not self-evident and the difference is what the channel plays in December.
const MODE_OPTIONS: readonly ChoiceOption<SeasonalMode>[] = [
  {
    value: "default",
    label: "Use built-in seasonal rotation",
    description:
      "Uses Loomarr's built-in holiday calendar. In-season titles play more often; other seasonal titles wait for their window.",
  },
  {
    value: "off",
    label: "Ignore holidays",
    description: "Holidays are ignored. Nothing is boosted or benched.",
  },
  {
    value: "exclusive",
    label: "Holiday-only channel",
    description: "The channel IS the holiday: only in-season titles play during the window.",
  },
];

// How many holiday episodes of an ordinary series the built-in rotation features per day.
const STRENGTH_OPTIONS: readonly ChoiceOption<SeasonalStrength>[] = [
  { value: "light", label: "Light", description: "About one holiday episode a day while a holiday is on." },
  {
    value: "default",
    label: "Featured",
    description: "A few holiday episodes a day while a holiday is on, and up to five on the day itself.",
  },
  {
    value: "heavy",
    label: "Heavy",
    description:
      "As many holiday episodes a day as the channel has without repeating one two days running, and all of them on the day itself.",
  },
];

const OFF_SEASON_OPTIONS: readonly ChoiceOption<SeasonalOffSeason>[] = [
  { value: "default", label: "Keep regular programming" },
  { value: "dark", label: "Go dark" },
];

/**
 * A channel's holiday-aware programming (programming-design §6): the seasonal mode, which holidays
 * count, and, for the built-in rotation, how strongly holiday episodes are featured and whether
 * they favour the evening. A control appears only where the scheduler reads it — strength and
 * evenings shape the built-in rotation alone, the out-of-season fallback applies to a holiday-only
 * channel alone, and the holiday list is hidden when holidays are ignored.
 */
const ChannelSeasonalFields = ({
  density = "pointer",
  mode,
  onModeChange,
  modeSource,
  holidays,
  includedHolidayIds,
  allHolidaysIncluded,
  onHolidayToggle,
  strength,
  onStrengthChange,
  strengthConsequence,
  favourEvenings,
  onFavourEveningsChange,
  offSeason,
  onOffSeasonChange,
}: ChannelSeasonalFieldsProps) => (
  <Surface backgroundColor="transparent" borderWidth={0} gap="section">
    <Surface backgroundColor="transparent" borderWidth={0} gap="inline">
      <ChoiceGroup
        density={density}
        label="Seasonal behavior"
        onValueChange={onModeChange}
        options={MODE_OPTIONS}
        value={mode}
      />
      <FieldHint density={density}>
        How this channel reacts to the calendar. Loomarr matches titles to a built-in holiday list; pick which
        holidays below.
      </FieldHint>
      {modeSource}
    </Surface>

    {mode !== "off" ? (
      <Surface
        aria-label="Included holidays"
        backgroundColor="transparent"
        borderWidth={0}
        gap="inline"
        role="group"
      >
        <Text density={density} textRole="label">
          Included holidays
        </Text>
        <Surface
          backgroundColor="transparent"
          borderWidth={0}
          flexDirection="row"
          flexWrap="wrap"
          gap="control"
        >
          {holidays.map((holiday) => (
            <Toggle
              checked={includedHolidayIds.includes(holiday.id)}
              density={density}
              key={holiday.id}
              label={holiday.label}
              onCheckedChange={(included) => onHolidayToggle(holiday.id, included)}
            />
          ))}
        </Surface>
        {allHolidaysIncluded ? (
          <FieldHint density={density}>All built-in holidays are included.</FieldHint>
        ) : null}
      </Surface>
    ) : null}

    {mode === "default" ? (
      <>
        <Surface backgroundColor="transparent" borderWidth={0} gap="inline">
          <ChoiceGroup
            density={density}
            label="Holiday strength"
            onValueChange={onStrengthChange}
            options={STRENGTH_OPTIONS}
            value={strength}
          />
          <FieldHint density={density}>
            How many holiday episodes of your regular shows to play while a holiday is on, like the Halloween
            episodes of a sitcom in October. One episode never comes back two days running.
          </FieldHint>
          {strengthConsequence ? <FieldHint density={density}>{strengthConsequence}</FieldHint> : null}
        </Surface>
        <Toggle
          checked={favourEvenings}
          density={density}
          description={
            favourEvenings
              ? "Holiday episodes play between 5 and 11 pm. Out of season they move out of the evening, unless that would bring an episode back too soon."
              : "Holiday episodes are spread across the whole day."
          }
          label="Favour evenings"
          onCheckedChange={onFavourEveningsChange}
        />
      </>
    ) : null}

    {mode === "exclusive" ? (
      <Surface backgroundColor="transparent" borderWidth={0} gap="inline">
        <ChoiceGroup
          density={density}
          label="Out of season"
          onValueChange={onOffSeasonChange}
          options={OFF_SEASON_OPTIONS}
          value={offSeason}
        />
        <FieldHint density={density}>What a holiday channel does the rest of the year.</FieldHint>
      </Surface>
    ) : null}
  </Surface>
);

export { ChannelSeasonalFields };
