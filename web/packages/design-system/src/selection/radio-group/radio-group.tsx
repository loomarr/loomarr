import { View } from "@tamagui/core";
import { type ComponentProps, createContext, type ReactNode, useContext, useRef, useState } from "react";
import { Pressable } from "react-native";

import { Surface } from "../../primitives";

type Focusable = { focus?: () => void };

interface RadioScopeValue {
  items: Map<string, Focusable>;
  select: (value: string) => void;
  value: string | undefined;
  values: readonly string[];
}

const RadioScope = createContext<RadioScopeValue | null>(null);

interface RadioGroupProps extends Omit<ComponentProps<typeof Surface>, "children" | "role"> {
  children: ReactNode;
  label: string;
  onValueChange: (value: string) => void;
  value: string | undefined;
  /** Every item's value in visual order: the order the arrow keys walk. */
  values: readonly string[];
}

/**
 * A single-choice group that behaves like a native radio group: one tab stop (the selected item,
 * or the first when none is selected), arrow keys move the choice and select it, and Space/Enter
 * select the focused item. Items draw their own look; `RadioItem` supplies the focus ring.
 */
const RadioGroup = ({ children, label, onValueChange, value, values, ...surface }: RadioGroupProps) => {
  const items = useRef(new Map<string, Focusable>());
  return (
    <RadioScope.Provider value={{ items: items.current, select: onValueChange, value, values }}>
      <Surface aria-label={label} role="radiogroup" {...surface}>
        {children}
      </Surface>
    </RadioScope.Provider>
  );
};

interface RadioItemProps {
  /** Receives the item's state so the look can follow it. */
  children: (state: { checked: boolean; focused: boolean }) => ReactNode;
  disabled?: boolean;
  label: string;
  /** Corner radius of the look drawn inside; the focus ring follows it. */
  ringRadius?: number;
  style?: ComponentProps<typeof Pressable>["style"];
  value: string;
}

const KEY_STEP: Record<string, 1 | -1> = { ArrowDown: 1, ArrowLeft: -1, ArrowRight: 1, ArrowUp: -1 };

const RadioItem = ({ children, disabled = false, label, ringRadius = 6, style, value }: RadioItemProps) => {
  const group = useContext(RadioScope);
  if (!group) throw new Error("RadioItem must be rendered inside a RadioGroup");
  const [focused, setFocused] = useState(false);
  const checked = group.value === value;
  const tabStop = group.values.includes(group.value ?? "") ? group.value : group.values[0];

  const move = (step: 1 | -1) => {
    const { values } = group;
    const next = values[(values.indexOf(value) + step + values.length) % values.length];
    if (next === undefined || next === value) return;
    group.select(next);
    group.items.get(next)?.focus?.();
  };

  return (
    <Pressable
      accessibilityLabel={label}
      accessibilityRole="radio"
      accessibilityState={{ checked, disabled }}
      aria-checked={checked}
      disabled={disabled}
      onBlur={() => setFocused(false)}
      onFocus={() => setFocused(true)}
      onPress={() => group.select(value)}
      ref={(node) => {
        if (node) group.items.set(value, node as unknown as Focusable);
        else group.items.delete(value);
      }}
      style={style}
      tabIndex={disabled ? -1 : tabStop === value ? 0 : -1}
      // react-native-web forwards keyboard events to the host element; native has no arrow keys.
      {...({
        onKeyDown: (event: { key: string; preventDefault: () => void }) => {
          const step = KEY_STEP[event.key];
          if (step === undefined) return;
          event.preventDefault();
          move(step);
        },
      } as object)}
    >
      <View>
        {children({ checked, focused })}
        {focused ? (
          <View
            borderColor="actionFocus"
            borderRadius={ringRadius + 3}
            borderWidth={2}
            bottom={-3}
            left={-3}
            pointerEvents="none"
            position="absolute"
            right={-3}
            top={-3}
          />
        ) : null}
      </View>
    </Pressable>
  );
};

export type { RadioGroupProps, RadioItemProps };
export { RadioGroup, RadioItem };
