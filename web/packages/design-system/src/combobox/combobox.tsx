import { View } from "@tamagui/core";
import { useState } from "react";

import { Field } from "../interaction";
import { PressableRow } from "../pressable-row";
import { Surface, Text } from "../primitives";
import type { ComboboxProps } from "./combobox.type";

// The native adapter: a phone, tablet or TV has no floating list, so the options open inline under
// the field, as rows (the web adapter, combobox.web.tsx, is Base UI's floating list). The list opens
// on focus or typing and closes when an option is picked. It does not close on blur: a press on a
// row would blur the field first and the row would never see it.
const Combobox = ({
  autoHighlight: _autoHighlight,
  density = "pointer",
  disabled = false,
  emptyText,
  errorText,
  id,
  inputValue,
  invalid = false,
  label,
  loading = false,
  onInputValueChange,
  onSelect,
  options,
  placeholder,
  stale = false,
  value = null,
}: ComboboxProps) => {
  const [open, setOpen] = useState(false);
  const showEmpty = !loading && !errorText && options.length === 0 && Boolean(emptyText);
  const hasContent = options.length > 0 || loading || Boolean(errorText) || showEmpty;

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
      {open && hasContent ? (
        <Surface gap="inline" level="raised" opacity={stale ? 0.5 : 1} padding="inline">
          {options.map((option) => (
            <PressableRow
              accessibilityLabel={
                option.description ? `${option.label}, ${option.description}` : option.label
              }
              disabled={option.disabled || stale}
              key={option.value}
              onPress={() => {
                setOpen(false);
                onSelect(option);
              }}
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
              {option.trailing ? (
                <Text density={density} textRole="metadata" tone="secondary">
                  {option.trailing}
                </Text>
              ) : null}
            </PressableRow>
          ))}
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
