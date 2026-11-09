import { CompactSelect, Surface } from "@loomarr/design-system";
import { NumberField } from "@loomarr/ui";
import { useState } from "react";

type DurationUnit = "days" | "hours" | "minutes" | "seconds";

const UNIT_SECONDS: Record<DurationUnit, number> = { days: 86400, hours: 3600, minutes: 60, seconds: 1 };

// The unit a stored amount reads in first: the largest offered unit that divides it evenly, so
// 14400 seconds opens as 4 hours. Zero says nothing about its unit, so it opens in the smallest.
const unitFor = (seconds: number, units: readonly DurationUnit[]): DurationUnit => {
  const largestFirst = [...units].sort((a, b) => UNIT_SECONDS[b] - UNIT_SECONDS[a]);
  const smallest = largestFirst[largestFirst.length - 1] ?? "seconds";
  if (seconds === 0) return smallest;
  return largestFirst.find((unit) => seconds % UNIT_SECONDS[unit] === 0) ?? smallest;
};

interface DurationFieldProps {
  id?: string;
  /** Names the unit picker ("<label> unit"). */
  label: string;
  /** The amount box's own name, for a box no <label htmlFor> points at. */
  amountLabel?: string;
  seconds: number;
  units: readonly DurationUnit[];
  onSeconds: (seconds: number) => void;
  disabled?: boolean;
  invalid?: boolean;
  describedBy?: string;
  labelledBy?: string;
}

// An amount and its unit. The unit is the person's choice and is kept while they type, so picking
// minutes for an empty (zero) amount stays minutes instead of snapping back to whatever unit the
// stored zero happens to divide by. The caller stores seconds and decides how they are written.
const DurationField = ({
  id,
  label,
  amountLabel,
  seconds,
  units,
  onSeconds,
  disabled,
  invalid,
  describedBy,
  labelledBy,
}: DurationFieldProps) => {
  const [unit, setUnit] = useState<DurationUnit>(() => unitFor(seconds, units));
  const amount = Number((seconds / UNIT_SECONDS[unit]).toFixed(3));
  return (
    <Surface backgroundColor="transparent" borderWidth={0} flexDirection="row" gap="inline">
      <NumberField
        accessibilityLabel={amountLabel}
        amount={amount}
        aria-describedby={describedBy}
        aria-labelledby={labelledBy}
        disabled={disabled}
        id={id}
        invalid={invalid}
        onAmount={(next) => onSeconds(next * UNIT_SECONDS[unit])}
      />
      <CompactSelect
        disabled={disabled}
        label={`${label} unit`}
        onValueChange={(next) => {
          setUnit(next);
          onSeconds(amount * UNIT_SECONDS[next]);
        }}
        options={units.map((value) => ({ label: value, value }))}
        value={unit}
        width={128}
      />
    </Surface>
  );
};

export type { DurationFieldProps, DurationUnit };
export { DurationField };
