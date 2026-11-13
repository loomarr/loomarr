import { RadioGroup, RadioItem, Surface, Text } from "@loomarr/design-system";
import type { ReactNode } from "react";
import { View } from "react-native";
import { group, thumb } from "../styles";

interface PickProps<V extends string> {
  columns?: number;
  /** Paused: real disabled radios. Only the picture dims; the names keep full contrast. */
  disabled?: boolean;
  label: string;
  /** Shown first in the row, before the cards (the mock's inline "Show as"). */
  lead?: ReactNode;
  onPick: (v: V) => void;
  options: readonly { label: string; value: V; visual?: ReactNode }[];
  value: V | undefined;
}

// A row of selectable cards (style thumbnails, scene buttons, "Show as"): one radio group.
// With `columns`, the cards sit in a fixed grid of equal tiles (the mock's .looks grid).
// Every choice on this panel is neutral-pressed when chosen (#1909, #1983): the focus fill, a
// foreground border and a 600 label. Amber is RadioItem's focus ring, drawn outside the border.
const Pick = <V extends string>({ columns, disabled, label, lead, onPick, options, value }: PickProps<V>) => {
  const values = options.map((o) => o.value);
  const card = (o: PickProps<V>["options"][number]) => (
    <RadioItem
      disabled={disabled}
      key={o.value}
      label={o.label}
      style={columns ? { flex: 1, minWidth: 0 } : undefined}
      value={o.value}
    >
      {({ checked }) => (
        <Surface
          alignItems={columns ? "stretch" : "center"}
          backgroundColor={checked ? "surfaceFocus" : "surfaceRaised"}
          borderColor={checked ? "contentPrimary" : "borderControl"}
          borderRadius="control"
          borderWidth={checked ? 2 : 1}
          flexGrow={columns ? 1 : undefined}
          gap={6}
          paddingHorizontal={columns ? 8 : 12}
          paddingVertical={8}
        >
          {columns ? (
            // The mock's .thumb: a dark gradient box that holds the bug, name left-aligned below.
            <View style={{ ...thumb, opacity: disabled ? 0.55 : 1 }}>{o.visual}</View>
          ) : o.visual ? (
            <View style={{ opacity: disabled ? 0.55 : 1 }}>{o.visual}</View>
          ) : null}
          <Text strong={checked} textRole="cardLabel">
            {o.label}
          </Text>
        </Surface>
      )}
    </RadioItem>
  );
  if (!columns) {
    return (
      <RadioGroup
        alignItems="center"
        label={label}
        onValueChange={(v) => onPick(v as V)}
        value={value}
        values={values}
        {...group}
      >
        {lead}
        {options.map(card)}
      </RadioGroup>
    );
  }
  // Rows of `columns` equal cells; the last row keeps empty cells so its tiles stay the same width.
  const rows = Array.from({ length: Math.ceil(options.length / columns) }, (_, i) =>
    options.slice(i * columns, (i + 1) * columns),
  );
  return (
    <RadioGroup
      borderWidth={0}
      gap={8}
      label={label}
      onValueChange={(v) => onPick(v as V)}
      value={value}
      values={values}
    >
      {rows.map((r) => (
        <View key={r[0]?.value} style={{ flexDirection: "row", gap: 8 }}>
          {r.map(card)}
          {Array.from({ length: columns - r.length }, (_, i) => (
            // biome-ignore lint/suspicious/noArrayIndexKey: an empty cell's only identity is its column.
            <View key={`empty-${i}`} style={{ flex: 1 }} />
          ))}
        </View>
      ))}
    </RadioGroup>
  );
};

export { Pick };
