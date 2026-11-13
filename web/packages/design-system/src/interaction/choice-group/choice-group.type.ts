import type { Density } from "../../tokens";

type ChoiceOption<Value extends string> = {
  description?: string;
  disabled?: boolean;
  label: string;
  value: Value;
};

type ChoiceGroupProps<Value extends string> = {
  density?: Density;
  label: string;
  onValueChange: (value: Value) => void;
  options: readonly ChoiceOption<Value>[];
  value: Value;
};

export type { ChoiceGroupProps, ChoiceOption };
