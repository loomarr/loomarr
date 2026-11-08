/// <reference lib="dom" />
import { Checkbox } from "@base-ui/react/checkbox";
import { Switch } from "@base-ui/react/switch";
import { type CSSProperties, useId, useState } from "react";

import { Icon } from "../icon";
import { icons } from "../icons";
import { Text } from "../primitives";
import { semanticRadius, semanticSpace, semanticTargets } from "../tokens";
import type { ToggleProps } from "./toggle.type";

// The web adapter, on Base UI Checkbox and Switch: the role and aria-checked sit on the drawn box,
// which drives a hidden native input. `id` lands on that input, so the wrapping <label> and any
// caller's own <label htmlFor> toggle it the way a browser checkbox would. The look is the native
// adapter's, value for value, read through the theme's CSS variables so light and dark both hold:
// a neutral-pressed box when checked (#1909) and the amber ring outside a gap from the keyboard.
const Toggle = ({
  "aria-describedby": describedBy,
  checked,
  density = "pointer",
  description,
  disabled = false,
  id,
  kind = "checkbox",
  label,
  labelHidden = false,
  onCheckedChange,
}: ToggleProps) => {
  const [ringed, setRinged] = useState(false);
  const descriptionId = useId();
  const shownDescription = description && !labelHidden;
  const switchWidth = density === "tv" ? 72 : density === "touch" ? 56 : 48;
  const controlSize = density === "tv" ? 36 : density === "touch" ? 28 : 24;
  const box: CSSProperties = {
    alignItems: "center",
    backgroundColor: checked ? "var(--surfaceElevated)" : "var(--surfaceCanvas)",
    borderColor: checked ? "var(--contentPrimary)" : "var(--borderControl)",
    borderRadius: kind === "switch" ? semanticRadius.round : semanticRadius.control,
    borderStyle: "solid",
    borderWidth: 1,
    boxSizing: "border-box",
    cursor: disabled ? "not-allowed" : "pointer",
    display: "inline-flex",
    flexShrink: 0,
    height: controlSize,
    justifyContent: kind === "switch" ? (checked ? "flex-end" : "flex-start") : "center",
    outline: ringed ? `${density === "tv" ? 3 : 2}px solid var(--actionFocus)` : "none",
    outlineOffset: 2,
    padding: kind === "switch" ? 3 : 0,
    width: kind === "switch" ? switchWidth : controlSize,
  };
  const control = {
    // The name is the label alone, as on native: the wrapping <label> would otherwise name the box
    // with its description too. The description is read after it instead.
    "aria-describedby":
      [shownDescription ? descriptionId : undefined, describedBy].filter(Boolean).join(" ") || undefined,
    "aria-label": label,
    checked,
    disabled,
    id,
    onBlur: () => setRinged(false),
    onCheckedChange: (next: boolean) => onCheckedChange(next),
    onFocus: (event: { currentTarget: HTMLElement }) =>
      setRinged(event.currentTarget.matches(":focus-visible")),
    style: box,
  };

  return (
    <label
      style={{
        alignItems: "center",
        cursor: disabled ? "not-allowed" : "pointer",
        display: "flex",
        gap: semanticSpace.control,
        minHeight: semanticTargets[density],
        opacity: disabled ? 0.55 : 1,
      }}
    >
      {kind === "switch" ? (
        <Switch.Root {...control}>
          <Switch.Thumb
            style={{
              backgroundColor: checked ? "var(--contentPrimary)" : "var(--contentSecondary)",
              borderRadius: semanticRadius.round,
              display: "block",
              height: controlSize - 8,
              width: controlSize - 8,
            }}
          />
        </Switch.Root>
      ) : (
        <Checkbox.Root {...control}>
          <Checkbox.Indicator style={{ display: "flex" }}>
            <Icon
              decorative
              glyph={icons.success}
              size={density === "tv" ? "control" : "compact"}
              tone="content"
            />
          </Checkbox.Indicator>
        </Checkbox.Root>
      )}
      {labelHidden ? null : (
        <span style={{ display: "flex", flex: 1, flexDirection: "column", gap: 2 }}>
          <Text density={density} textRole="label">
            {label}
          </Text>
          {shownDescription ? (
            <Text density={density} id={descriptionId} textRole="hint">
              {description}
            </Text>
          ) : null}
        </span>
      )}
    </label>
  );
};

export { Toggle };
