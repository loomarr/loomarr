import { Action, Surface, Text } from "@loomarr/design-system";
import { useEffect, useId, useState } from "react";

import { YearField } from "../year-field";
import type { YearRange, YearRangeListFieldProps } from "./year-range-list-field.type";

const YEAR_MIN = 1900;
const YEAR_MAX = 2099;
const RANGE_ERROR = `Enter whole years from ${YEAR_MIN} to ${YEAR_MAX}, with From no later than To.`;

const isValidYearRange = (range: { from?: number; to?: number }): range is YearRange =>
  range.from !== undefined &&
  range.to !== undefined &&
  Number.isInteger(range.from) &&
  Number.isInteger(range.to) &&
  range.from >= YEAR_MIN &&
  range.from <= YEAR_MAX &&
  range.to >= YEAR_MIN &&
  range.to <= YEAR_MAX &&
  range.from <= range.to;

// The valid ranges, sorted, with touching and overlapping ones merged (1990–1999 and 2000–2005 are
// one span, since the ranges are inclusive).
const normalizeYearRanges = (ranges: readonly { from?: number; to?: number }[] | undefined): YearRange[] => {
  const valid = (ranges ?? []).filter(isValidYearRange).sort((a, b) => a.from - b.from || a.to - b.to);
  return valid.reduce<YearRange[]>((merged, range) => {
    const previous = merged.at(-1);
    if (previous && range.from <= previous.to + 1) previous.to = Math.max(previous.to, range.to);
    else merged.push({ from: range.from, to: range.to });
    return merged;
  }, []);
};

const YearRangeRow = ({
  disabled,
  idBase,
  index,
  label,
  onCommit,
  onRemove,
  range,
}: {
  disabled?: boolean;
  idBase: string;
  index: number;
  label: string;
  onCommit: (index: number, next: { from: number; to: number }) => void;
  onRemove: (index: number) => void;
  range: YearRange;
}) => {
  const [draft, setDraft] = useState({ from: String(range.from), to: String(range.to) });

  // An invalid endpoint stays visible until its partner makes the pair valid. The committed range is
  // the reset boundary, so a draft neither survives an outside replacement nor slides onto another
  // row when the list changes.
  useEffect(() => {
    setDraft({ from: String(range.from), to: String(range.to) });
  }, [range.from, range.to]);

  // The visible text stays short and still starts the accessible name, as a Reset button's does.
  const where = `(${label}, range ${index + 1})`;
  const commit = () => onCommit(index, { from: Number(draft.from), to: Number(draft.to) });
  return (
    <Surface
      alignItems="flex-end"
      backgroundColor="transparent"
      borderWidth={0}
      flexDirection="row"
      flexWrap="wrap"
      gap="inline"
    >
      {/* The From/To pair never wraps apart: on a phone only Remove drops to the next line. */}
      <Surface backgroundColor="transparent" borderWidth={0} flexDirection="row" gap="inline">
        <YearField
          accessibilityLabel={`From year ${where}`}
          disabled={disabled}
          id={`${idBase}-${index}-from`}
          label="From year"
          onBlur={commit}
          onChangeText={(from) => setDraft((current) => ({ ...current, from }))}
          value={draft.from}
        />
        <YearField
          accessibilityLabel={`To year ${where}`}
          disabled={disabled}
          id={`${idBase}-${index}-to`}
          label="To year"
          onBlur={commit}
          onChangeText={(to) => setDraft((current) => ({ ...current, to }))}
          value={draft.to}
        />
      </Surface>
      <Action
        accessibilityLabel={`Remove ${where}`}
        disabled={disabled}
        onPress={() => onRemove(index)}
        variant="ghost"
      >
        Remove
      </Action>
    </Surface>
  );
};

/**
 * A list of whole-year ranges: one From/To row per range, committed when focus leaves the row, and a
 * blank row to add another. The era windows of a filler selection and the date axes of a channel
 * policy edit through it, so both read the same to a screen reader and validate the same way.
 */
const YearRangeListField = ({
  disabled,
  emptyText,
  label,
  maxRanges,
  onChange,
  ranges,
}: YearRangeListFieldProps) => {
  const idBase = useId();
  const [draft, setDraft] = useState({ from: "", to: "" });
  const [error, setError] = useState<string>();

  const commit = (index: number, next: { from: number; to: number }) => {
    if (!isValidYearRange(next)) {
      setError(RANGE_ERROR);
      return;
    }
    setError(undefined);
    const changed = [...ranges];
    changed[index] = next;
    onChange(normalizeYearRanges(changed));
  };

  const add = () => {
    const next = { from: Number(draft.from), to: Number(draft.to) };
    if (!draft.from || !draft.to || !isValidYearRange(next)) {
      setError(RANGE_ERROR);
      return;
    }
    setError(undefined);
    setDraft({ from: "", to: "" });
    onChange(normalizeYearRanges([...ranges, next]));
  };

  return (
    <Surface backgroundColor="transparent" borderWidth={0} gap="inline">
      {ranges.length === 0 && emptyText ? (
        <Text textRole="caption" tone="secondary">
          {emptyText}
        </Text>
      ) : (
        ranges.map((range, index) => (
          <YearRangeRow
            disabled={disabled}
            idBase={idBase}
            index={index}
            key={`${range.from}-${range.to}`}
            label={label}
            onCommit={commit}
            onRemove={(row) => onChange(ranges.filter((_, current) => current !== row))}
            range={range}
          />
        ))
      )}
      {maxRanges === undefined || ranges.length < maxRanges ? (
        <Surface
          alignItems="flex-end"
          backgroundColor="transparent"
          borderWidth={0}
          flexDirection="row"
          flexWrap="wrap"
          gap="inline"
        >
          <Surface backgroundColor="transparent" borderWidth={0} flexDirection="row" gap="inline">
            <YearField
              accessibilityLabel={`${label} new range from`}
              disabled={disabled}
              onChangeText={(from) => setDraft((current) => ({ ...current, from }))}
              placeholder="From"
              value={draft.from}
            />
            <YearField
              accessibilityLabel={`${label} new range to`}
              disabled={disabled}
              onChangeText={(to) => setDraft((current) => ({ ...current, to }))}
              placeholder="To"
              value={draft.to}
            />
          </Surface>
          <Action
            accessibilityLabel={`Add range for ${label}`}
            disabled={disabled}
            onPress={add}
            variant="outline"
          >
            Add range
          </Action>
        </Surface>
      ) : null}
      {error ? (
        <Surface backgroundColor="transparent" borderWidth={0} role="alert">
          <Text textRole="caption" tone="danger">
            {error}
          </Text>
        </Surface>
      ) : null}
    </Surface>
  );
};

export { isValidYearRange, normalizeYearRanges, YEAR_MAX, YEAR_MIN, YearRangeListField };
