import { View } from "@tamagui/core";
import { useState } from "react";

import { Field } from "../interaction";
import { PressableRow } from "../pressable-row";
import { Surface } from "../primitives";
import { Text } from "../text";
import type { Density } from "../tokens";
import type { ComboboxOption, ComboboxProps } from "./combobox.type";

const OptionRow = ({
  density,
  onPick,
  option,
  stale,
  value,
}: {
  density: Density;
  onPick: (option: ComboboxOption) => void;
  option: ComboboxOption;
  stale: boolean;
  value: string | null;
}) => (
  <PressableRow
    accessibilityLabel={option.description ? `${option.label}, ${option.description}` : option.label}
    disabled={option.disabled || stale}
    onPress={() => onPick(option)}
    selected={option.value === value}
  >
    <View flex={1} minWidth={0}>
      <Text density={density} textRole="body">
        {option.label}
      </Text>
      {option.description ? (
        <Text density={density} textRole="metadata" tone="secondary">
          {option.description}
        </Text>
      ) : null}
    </View>
    {typeof option.trailing === "string" ? (
      <Text density={density} textRole="metadata" tone="secondary">
        {option.trailing}
      </Text>
    ) : (
      (option.trailing ?? null)
    )}
  </PressableRow>
);

// The native adapter: a phone, tablet or TV has no floating list, so the options open inline under
// the field, as rows (the web adapter, combobox.web.tsx, is Base UI's floating list). The list opens
// on focus or typing and closes when an option is picked. It does not close on blur: a press on a
// row would blur the field first and the row would never see it. `inline` keeps it open. Focus moves
// by the platform's own row focus, so the web keyboard additions (Home, End, Escape, the inline
// header's adornments) have no native counterpart.
const Combobox = ({
  autoHighlight: _autoHighlight,
  density = "pointer",
  disabled = false,
  emptyText,
  endAdornment: _endAdornment,
  errorText,
  groups,
  id,
  inline = false,
  inputValue,
  invalid = false,
  label,
  listLabel: _listLabel,
  loading = false,
  onEscape: _onEscape,
  onInputValueChange,
  onSelect,
  options,
  placeholder,
  startAdornment: _startAdornment,
  stale = false,
  value = null,
}: ComboboxProps) => {
  const [open, setOpen] = useState(false);
  const drawnGroups = groups?.filter((group) => group.options.length > 0);
  const flat = drawnGroups ? drawnGroups.flatMap((group) => group.options) : (options ?? []);
  const showEmpty = !loading && !errorText && flat.length === 0 && Boolean(emptyText);
  const hasContent = flat.length > 0 || loading || Boolean(errorText) || showEmpty;
  const pick = (option: ComboboxOption) => {
    if (!inline) setOpen(false);
    onSelect(option);
  };
  const row = (option: ComboboxOption) => (
    <OptionRow
      density={density}
      key={option.value}
      onPick={pick}
      option={option}
      stale={stale}
      value={value}
    />
  );

  return (
    <View gap="inline" width="100%">
      <Field
        accessibilityLabel={label}
        autoCapitalize="none"
        autoCorrect={false}
        density={density}
        disabled={disabled}
        id={id}
        invalid={invalid}
        onChangeText={(next) => {
          onInputValueChange(next);
          setOpen(true);
        }}
        onFocus={() => setOpen(true)}
        placeholder={placeholder}
        value={inputValue}
      />
      {(open || inline) && hasContent ? (
        <Surface gap="inline" level="raised" opacity={stale ? 0.5 : 1} padding="inline">
          {drawnGroups
            ? drawnGroups.map((group) => (
                <View accessibilityLabel={group.label} gap="inline" key={group.id}>
                  <Text density={density} textRole="metadata" tone="secondary">
                    {group.label}
                  </Text>
                  {group.options.map(row)}
                </View>
              ))
            : flat.map(row)}
          {loading ? (
            <Text aria-live="polite" density={density} textRole="caption" tone="secondary">
              Searching…
            </Text>
          ) : null}
          {showEmpty ? (
            <Text aria-live="polite" density={density} textRole="caption" tone="secondary">
              {emptyText}
            </Text>
          ) : null}
          {errorText ? (
            <Text accessibilityRole="alert" density={density} textRole="caption" tone="danger">
              {errorText}
            </Text>
          ) : null}
        </Surface>
      ) : null}
    </View>
  );
};

export { Combobox };
