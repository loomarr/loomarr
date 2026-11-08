/// <reference lib="dom" />
import { useState } from "react";

import { semanticRadius, semanticSpace, semanticTargets, typography } from "../tokens";
import type { CompactSelectProps } from "./compact-select.type";

// The web adapter: a native <select>, as the mocks draw every toolbar and settings-row choice, so
// the browser owns the open list, typeahead and the phone's own picker. The closed box is the field
// select's (SelectControl's `field` look) value for value, read through the theme's CSS variables so
// light and dark both hold, with the amber ring outside a gap from the keyboard.
const CompactSelect = <Value extends string>({
  "aria-describedby": describedBy,
  "aria-labelledby": labelledBy,
  density = "pointer",
  disabled = false,
  id,
  invalid = false,
  label,
  onValueChange,
  options,
  value,
  width,
}: CompactSelectProps<Value>) => {
  const [ringed, setRinged] = useState(false);
  const text = typography[density].compact;
  return (
    <select
      aria-describedby={describedBy}
      aria-invalid={invalid || undefined}
      aria-label={labelledBy ? undefined : label}
      aria-labelledby={labelledBy}
      disabled={disabled}
      id={id}
      onBlur={() => setRinged(false)}
      onChange={(event) => onValueChange(event.currentTarget.value as Value)}
      onFocus={(event) => setRinged(event.currentTarget.matches(":focus-visible"))}
      style={{
        backgroundColor: "var(--surfaceCanvas)",
        borderColor: invalid ? "var(--stateDanger)" : "var(--borderControl)",
        borderRadius: semanticRadius.control,
        borderStyle: "solid",
        borderWidth: 1,
        boxSizing: "border-box",
        color: "var(--contentPrimary)",
        cursor: disabled ? "not-allowed" : "pointer",
        fontFamily: typography.family.body.web,
        fontSize: text.size,
        lineHeight: `${text.lineHeight}px`,
        maxWidth: "100%",
        minHeight: semanticTargets[density],
        opacity: disabled ? 0.62 : 1,
        outline: ringed ? `${density === "tv" ? 3 : 2}px solid var(--actionFocus)` : "none",
        outlineOffset: 2,
        paddingInline: semanticSpace.control,
        width: width ?? "100%",
      }}
      value={value}
    >
      {options.map((option) => (
        <option disabled={option.disabled} key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  );
};

export { CompactSelect };
