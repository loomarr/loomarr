import type { SettingPreset } from "@loomarr/api/models/settingPreset";
import { Field, Surface } from "@loomarr/design-system";
import { useState } from "react";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
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

interface CronFieldProps {
  id: string;
  label: string;
  value: string;
  presets: readonly SettingPreset[];
  onChange: (value: string) => void;
  disabled?: boolean;
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

  return (
    <div className="flex flex-col gap-2">
      <Select
        value={mode}
        disabled={disabled}
        onValueChange={(next) => {
          if (next === CUSTOM) {
            setCustom(true);
            return;
          }
          setCustom(false);
          onChange(next === DAILY ? dailyCron(daily ?? DEFAULT_DAILY) : next);
        }}
      >
        <SelectTrigger
          id={id}
          aria-describedby={describedBy}
          aria-labelledby={labelledBy}
          aria-invalid={invalid ? "true" : undefined}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {presets.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
          <SelectItem value={DAILY}>Every day at a time I choose</SelectItem>
          <SelectItem value={CUSTOM}>Custom schedule</SelectItem>
        </SelectContent>
      </Select>

      {mode === DAILY && daily && (
        <div className="flex gap-2">
          <Select
            value={String(daily.hour)}
            disabled={disabled}
            onValueChange={(hour) => onChange(dailyCron({ ...daily, hour: Number(hour) }))}
          >
            <SelectTrigger aria-label={`${label} hour`}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {HOURS.map((hour) => (
                <SelectItem key={hour} value={String(hour)}>
                  {hourLabel(hour)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select
            value={String(daily.minute)}
            disabled={disabled}
            onValueChange={(minute) => onChange(dailyCron({ ...daily, minute: Number(minute) }))}
          >
            <SelectTrigger aria-label={`${label} minute`} className="w-28 shrink-0">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {minutes.map((minute) => (
                <SelectItem key={minute} value={String(minute)}>
                  {`:${twoDigits(minute)}`}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}

      {mode === CUSTOM && (
        <Surface backgroundColor="transparent" borderWidth={0} flexGrow={1} flexShrink={1} minWidth={0}>
          <Field
            aria-label={`${label} custom schedule`}
            autoCapitalize="none"
            autoCorrect={false}
            disabled={disabled}
            invalid={invalid}
            onChangeText={onChange}
            value={value}
          />
        </Surface>
      )}
    </div>
  );
};

export type { CronFieldProps };
export { CronField };
