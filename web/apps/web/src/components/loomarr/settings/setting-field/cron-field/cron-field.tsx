import type { SettingPreset } from "@loomarr/api/models/settingPreset";
import { CompactSelect, Field, Surface } from "@loomarr/design-system";
import { useState } from "react";
import { dailyCron, parseDaily } from "./cron-time";

// Two choices that are not registry presets. Neither can collide with a cron: a cron has six
// space-separated fields and these are single words.
const DAILY = "daily";
const CUSTOM = "custom";

const DEFAULT_DAILY = { hour: 3, minute: 0 } as const;
const HOURS = [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23];
const MINUTES = [0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55];

const hourLabel = (hour: number): string =>
  new Intl.DateTimeFormat(undefined, { hour: "numeric" }).format(new Date(2000, 0, 1, hour));

const twoDigits = (n: number): string => String(n).padStart(2, "0");

const HOUR_OPTIONS = HOURS.map((hour) => ({ value: String(hour), label: hourLabel(hour) }));

interface CronFieldProps {
  id: string;
  label: string;
  value: string;
  presets: readonly SettingPreset[];
  onChange: (value: string) => void;
  disabled?: boolean;
  /** A schedule the environment pins: locked, but shown at full strength (#2759). */
  readOnly?: boolean;
  invalid?: boolean;
  describedBy?: string;
  labelledBy?: string;
}

// A schedule as an admin thinks of it: a named schedule from the registry, or every day at a time
// they pick. A schedule that is neither (an advanced install) keeps its raw expression, behind
// "Custom schedule", rather than being rewritten.
const CronField = ({
  id,
  label,
  value,
  presets,
  onChange,
  disabled,
  readOnly,
  invalid,
  describedBy,
  labelledBy,
}: CronFieldProps) => {
  const [custom, setCustom] = useState(false);
  const preset = presets.find((option) => option.value === value.trim());
  const daily = parseDaily(value);
  const mode = custom ? CUSTOM : preset ? preset.value : daily ? DAILY : CUSTOM;
  const minutes =
    daily && !MINUTES.includes(daily.minute) ? [...MINUTES, daily.minute].sort((a, b) => a - b) : MINUTES;
  const modeOptions = [
    ...presets.map((option) => ({ value: option.value, label: option.label })),
    { value: DAILY, label: "Every day at a time I choose" },
    { value: CUSTOM, label: "Custom schedule" },
  ];

  return (
    <Surface backgroundColor="transparent" borderWidth={0} gap="inline">
      <CompactSelect
        aria-describedby={describedBy}
        aria-labelledby={labelledBy}
        disabled={disabled}
        id={id}
        invalid={invalid}
        label={label}
        onValueChange={(next) => {
          if (next === CUSTOM) {
            setCustom(true);
            return;
          }
          setCustom(false);
          onChange(next === DAILY ? dailyCron(daily ?? DEFAULT_DAILY) : next);
        }}
        options={modeOptions}
        readOnly={readOnly}
        value={mode}
      />

      {mode === DAILY && daily && (
        <Surface backgroundColor="transparent" borderWidth={0} flexDirection="row" gap="inline">
          <Surface backgroundColor="transparent" borderWidth={0} flexGrow={1} flexShrink={1} minWidth={0}>
            <CompactSelect
              disabled={disabled}
              label={`${label} hour`}
              onValueChange={(hour) => onChange(dailyCron({ ...daily, hour: Number(hour) }))}
              options={HOUR_OPTIONS}
              readOnly={readOnly}
              value={String(daily.hour)}
            />
          </Surface>
          <CompactSelect
            disabled={disabled}
            label={`${label} minute`}
            onValueChange={(minute) => onChange(dailyCron({ ...daily, minute: Number(minute) }))}
            options={minutes.map((minute) => ({ value: String(minute), label: `:${twoDigits(minute)}` }))}
            readOnly={readOnly}
            value={String(daily.minute)}
            width={112}
          />
        </Surface>
      )}

      {mode === CUSTOM && (
        <Surface backgroundColor="transparent" borderWidth={0} flexGrow={1} flexShrink={1} minWidth={0}>
          <Field
            aria-label={`${label} custom schedule`}
            autoCapitalize="none"
            autoCorrect={false}
            // DS Field has no read-only state yet, so a pinned custom expression stays disabled.
            disabled={disabled || readOnly}
            invalid={invalid}
            onChangeText={onChange}
            value={value}
          />
        </Surface>
      )}
    </Surface>
  );
};

export type { CronFieldProps };
export { CronField };
