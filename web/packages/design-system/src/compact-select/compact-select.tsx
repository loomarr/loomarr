import { useState } from "react";

import { SelectControl } from "../selection";
import type { CompactSelectProps } from "./compact-select.type";

// The native adapter: a phone, tablet or TV has no compact toolbar, so it opens the field select's
// own inline choice list. The web adapter (compact-select.web.tsx) is a native <select>.
const CompactSelect = <Value extends string>({
  density,
  disabled,
  label,
  onValueChange,
  options,
  value,
}: CompactSelectProps<Value>) => {
  const [open, setOpen] = useState(false);
  return (
    <SelectControl
      appearance="field"
      density={density}
      disabled={disabled}
      label={label}
      onOpenChange={setOpen}
      onValueChange={onValueChange}
      open={open}
      options={options}
      value={value}
    />
  );
};

export { CompactSelect };
