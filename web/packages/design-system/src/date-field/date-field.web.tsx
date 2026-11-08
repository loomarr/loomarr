/// <reference lib="dom" />
import { useId, useState } from "react";

import { Text } from "../primitives";
import { semanticRadius, semanticSpace, semanticTargets, typography } from "../tokens";
import type { DateFieldProps } from "./date-field.type";

// The web adapter: the browser's own date input, so the calendar, the locale's order and the
// keyboard stepping are the browser's. The box is the Field's, value for value — a 2px control
// border that turns amber and 3px on focus, the data face at body size — read through the theme's
// CSS variables so light and dark both hold.
const DateField = ({
  density = "pointer",
  description,
  disabled = false,
  id,
  kind = "date",
  label,
  onValueChange,
  value,
  width,
}: DateFieldProps) => {
  const [focused, setFocused] = useState(false);
  const generatedId = useId();
  const inputId = id ?? generatedId;
  const descriptionId = `${inputId}-description`;
  return (
    <div
      style={{ display: "flex", flexDirection: "column", gap: semanticSpace.inline, width: width ?? "100%" }}
    >
      <Text as="label" density={density} htmlFor={inputId} textRole="label">
        {label}
      </Text>
      <input
        aria-describedby={description ? descriptionId : undefined}
        disabled={disabled}
        id={inputId}
        onBlur={() => setFocused(false)}
        onChange={(event) => onValueChange(event.currentTarget.value)}
        onFocus={() => setFocused(true)}
        style={{
          backgroundColor: disabled ? "var(--surfaceRaised)" : "var(--surfaceCanvas)",
          borderColor: focused ? "var(--actionFocus)" : "var(--borderControl)",
          borderRadius: semanticRadius.control,
          borderStyle: "solid",
          borderWidth: focused ? 3 : 2,
          boxSizing: "border-box",
          color: "var(--contentPrimary)",
          colorScheme: "light dark",
          fontFamily: typography.family.data.web,
          fontSize: typography[density].body.size,
          minHeight: semanticTargets[density],
          opacity: disabled ? 0.62 : 1,
          outline: "none",
          paddingInline: semanticSpace.control,
          width: "100%",
        }}
        type={kind === "datetime" ? "datetime-local" : "date"}
        value={value}
      />
      {description ? (
        <Text density={density} id={descriptionId} textRole="metadata">
          {description}
        </Text>
      ) : null}
    </div>
  );
};

export { DateField };
