import { View } from "@tamagui/core";

import { Field } from "../interaction";
import type { DateFieldProps } from "./date-field.type";

// The native adapter: a Field that takes the same text the web input produces, with the format as
// its placeholder. A phone or TV that needs a date picker gets one here, not in a caller.
const DateField = ({
  density,
  description,
  disabled,
  id,
  kind = "date",
  label,
  onValueChange,
  value,
  width,
}: DateFieldProps) => (
  <View width={width ?? "100%"}>
    <Field
      autoCapitalize="none"
      autoCorrect={false}
      density={density}
      description={description}
      disabled={disabled}
      id={id}
      keyboardType="numbers-and-punctuation"
      label={label}
      maxLength={kind === "datetime" ? 16 : 10}
      onChangeText={onValueChange}
      placeholder={kind === "datetime" ? "YYYY-MM-DDTHH:MM" : "YYYY-MM-DD"}
      value={value}
    />
  </View>
);

export { DateField };
