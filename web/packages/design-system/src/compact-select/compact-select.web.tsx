/// <reference lib="dom" />
import { useFocusLook } from "../focus-look";
import { semanticRadius, semanticSpace, semanticTargets, typography } from "../tokens";
import type { CompactSelectProps } from "./compact-select.type";

// The web adapter: a native <select>, as the mocks draw every toolbar and settings-row choice, so
// the browser owns the open list, typeahead and the phone's own picker. The closed box is the field
// select's (SelectControl's `field` look) value for value, read through the theme's CSS variables so
// light and dark both hold, with the field's amber border on focus.
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
  readOnly = false,
  value,
  width,
}: CompactSelectProps<Value>) => {
  // A <select> has no read-only state, so a locked one is disabled but keeps full strength.
  const locked = disabled || readOnly;
  const { focused, onBlur, onFocus } = useFocusLook();
  const borderWidth = focused ? 2 : 1;
  const text = typography[density].compact;
  return (
    <select
      aria-describedby={describedBy}
      aria-invalid={invalid || undefined}
      aria-label={labelledBy ? undefined : label}
      aria-labelledby={labelledBy}
      disabled={locked}
      id={id}
      onBlur={onBlur}
      onChange={(event) => onValueChange(event.currentTarget.value as Value)}
      onFocus={onFocus}
      style={{
        backgroundColor: "var(--surfaceCanvas)",
        // Amber is focus, whatever else the field says: an invalid select shows its red again on blur.
        borderColor: focused ? "var(--actionFocus)" : invalid ? "var(--stateDanger)" : "var(--borderControl)",
        borderRadius: semanticRadius.control,
        borderStyle: "solid",
        borderWidth,
        boxSizing: "border-box",
        color: "var(--contentPrimary)",
        cursor: disabled ? "not-allowed" : readOnly ? "default" : "pointer",
        fontFamily: typography.family.body.web,
        fontSize: text.size,
        lineHeight: `${text.lineHeight}px`,
        maxWidth: "100%",
        minHeight: semanticTargets[density],
        opacity: disabled ? 0.62 : 1,
        // One ring, the border. A transparent outline is what forced colours paint instead.
        outline: focused ? "2px solid transparent" : "none",
        // The wider border takes its extra pixel from the padding, so the words stay put.
        paddingInline: semanticSpace.control - (borderWidth - 1),
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
