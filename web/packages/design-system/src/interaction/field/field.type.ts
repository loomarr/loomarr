import type { ComponentProps } from "react";
import type { TextInput } from "react-native";

import type { Density } from "../../tokens";

type FieldProps = Omit<ComponentProps<typeof TextInput>, "editable"> & {
  density?: Density;
  description?: string;
  disabled?: boolean;
  error?: string;
  invalid?: boolean;
  label?: string;
};

export type { FieldProps };
