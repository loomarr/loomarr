import { View } from "@tamagui/core";
import type { ReactNode } from "react";

import { Action, ChoiceGroup } from "../interaction";
import { Surface, Text } from "../primitives";
import type { Density } from "../tokens";

type TabOption<Value extends string> = {
  disabled?: boolean;
  label: string;
  value: Value;
};

interface TabsProps<Value extends string> {
  density?: Density;
  label: string;
  onValueChange: (value: Value) => void;
  options: readonly TabOption<Value>[];
  value: Value;
}

const Tabs = <Value extends string>({
  density = "pointer",
  label,
  onValueChange,
  options,
  value,
}: TabsProps<Value>) => (
  <View aria-label={label} flexDirection="row" gap="$inline" role="tablist">
    {options.map((option) => {
      const selected = option.value === value;
      return (
        <Action
          accessibilityRole="tab"
          density={density}
          disabled={option.disabled}
          key={option.value}
          onPress={() => onValueChange(option.value)}
          selected={selected}
          tabIndex={selected ? 0 : -1}
          tone="secondary"
        >
          {option.label}
        </Action>
      );
    })}
  </View>
);

type MenuItem<Value extends string> = {
  disabled?: boolean;
  label: string;
  tone?: "danger" | "primary" | "secondary";
  value: Value;
};

interface MenuListProps<Value extends string> {
  density?: Density;
  items: readonly MenuItem<Value>[];
  label: string;
  onSelect: (value: Value) => void;
}

const MenuList = <Value extends string>({
  density = "pointer",
  items,
  label,
  onSelect,
}: MenuListProps<Value>) => (
  <Surface aria-label={label} gap="$inline" padding="$inline" role="menu">
    {items.map((item) => (
      <Action
        accessibilityRole="menuitem"
        density={density}
        disabled={item.disabled}
        key={item.value}
        onPress={() => onSelect(item.value)}
        tone={item.tone ?? "secondary"}
      >
        {item.label}
      </Action>
    ))}
  </Surface>
);

type SelectOption<Value extends string> = {
  description?: string;
  disabled?: boolean;
  label: string;
  value: Value;
};

interface SelectControlProps<Value extends string> {
  density?: Density;
  disabled?: boolean;
  label: string;
  onOpenChange: (open: boolean) => void;
  onValueChange: (value: Value) => void;
  open: boolean;
  options: readonly SelectOption<Value>[];
  value: Value;
}

const SelectControl = <Value extends string>({
  density = "pointer",
  disabled = false,
  label,
  onOpenChange,
  onValueChange,
  open,
  options,
  value,
}: SelectControlProps<Value>) => {
  const selected = options.find((option) => option.value === value);
  return (
    <View gap="$inline" width="100%">
      <Text density={density} textRole="label">
        {label}
      </Text>
      <Action
        accessibilityLabel={`${label}, ${selected?.label ?? "No selection"}`}
        accessibilityRole="button"
        aria-expanded={open}
        density={density}
        disabled={disabled}
        onPress={() => onOpenChange(!open)}
        tone="secondary"
      >
        {selected?.label ?? "Choose an option"}
      </Action>
      {open ? (
        <Surface padding="$control">
          <ChoiceGroup
            density={density}
            label={`${label} options`}
            onValueChange={(next) => {
              onValueChange(next);
              onOpenChange(false);
            }}
            options={options}
            value={value}
          />
        </Surface>
      ) : null}
    </View>
  );
};

interface HintProps {
  children: ReactNode;
  content: string;
  density?: Density;
  visible: boolean;
}

const Hint = ({ children, content, density = "pointer", visible }: HintProps) => (
  <View alignItems="flex-start" gap="$inline">
    {children}
    {visible ? (
      <Surface level="overlay" maxWidth={density === "tv" ? 520 : 320} padding="$inline" role="tooltip">
        <Text density={density} textRole="metadata" tone="primary">
          {content}
        </Text>
      </Surface>
    ) : null}
  </View>
);

export type { HintProps, MenuItem, MenuListProps, SelectControlProps, SelectOption, TabOption, TabsProps };
export { Hint, MenuList, SelectControl, Tabs };
