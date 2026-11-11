/// <reference lib="dom" />
import { Combobox as ComboboxPrimitive } from "@base-ui/react/combobox";

import { useFocusLook } from "../focus-look";
import { Icon, icons } from "../icon";
import { semanticRadius, semanticSpace, semanticTargets, typography } from "../tokens";
import type { ComboboxOption, ComboboxProps } from "./combobox.type";

const shadow = "0 10px 15px -3px rgb(0 0 0 / 0.1), 0 4px 6px -4px rgb(0 0 0 / 0.1)";

const statusStyle = {
  color: "var(--contentSecondary)",
  fontSize: 13,
  lineHeight: "18px",
  padding: `${semanticSpace.inline}px ${semanticSpace.control}px`,
} as const;

// The web adapter, on Base UI Combobox: the combobox role, aria-activedescendant (DOM focus stays in
// the input while the highlight moves), arrow keys, Home and End, Escape, and focus staying put when
// an option is pressed. Base UI draws nothing, so the field is Field's look value for value (the
// one amber border for focus, the data face) and the list is Menu's surface, read through the
// theme's CSS variables so light and dark both hold. The list portals, so a field inside a
// transformed row or a clipped card cannot cut it off.
//
// The combobox does not filter and does not own the text: `options` arrive already narrowed and the
// caller keeps `inputValue`, so a debounced server search and a local list use the same control.
const Combobox = ({
  "aria-describedby": describedBy,
  "aria-labelledby": labelledBy,
  autoHighlight = false,
  density = "pointer",
  disabled = false,
  emptyText,
  errorText,
  id,
  inputValue,
  invalid = false,
  label,
  loading = false,
  onInputValueChange,
  onSelect,
  options,
  placeholder,
  portalContainer,
  stale = false,
  value = null,
}: ComboboxProps) => {
  const { focused, ...focusHandlers } = useFocusLook();
  const text = typography[density].body;
  const selected = options.find((option) => option.value === value) ?? null;
  const showEmpty = !loading && !errorText && options.length === 0 && Boolean(emptyText);
  const hasContent = options.length > 0 || loading || Boolean(errorText) || showEmpty;

  return (
    <ComboboxPrimitive.Root<ComboboxOption>
      autoHighlight={autoHighlight}
      disabled={disabled}
      filter={null}
      inputValue={inputValue}
      isItemEqualToValue={(item, chosen) => item.value === chosen.value}
      items={options}
      itemToStringLabel={(option) => option.label}
      onInputValueChange={(next, details) => {
        // Only the user's own typing reaches the caller. Base UI also writes the input itself: the
        // label of a picked option, and a clear whenever the list closes. The caller hears a pick
        // through `onSelect` and keeps what was typed when the list closes.
        if (details.reason === "input-change") onInputValueChange(next);
      }}
      onValueChange={(option) => {
        if (option) onSelect(option);
      }}
      value={selected}
    >
      <ComboboxPrimitive.Input
        {...focusHandlers}
        aria-describedby={describedBy}
        aria-invalid={invalid || undefined}
        aria-label={labelledBy ? undefined : label}
        aria-labelledby={labelledBy}
        autoComplete="off"
        id={id}
        placeholder={placeholder}
        style={{
          backgroundColor: disabled ? "var(--surfaceRaised)" : "var(--surfaceCanvas)",
          border: `${focused ? 3 : 2}px solid ${
            invalid ? "var(--stateDanger)" : focused ? "var(--actionFocus)" : "var(--borderControl)"
          }`,
          borderRadius: semanticRadius.control,
          boxSizing: "border-box",
          color: "var(--contentPrimary)",
          cursor: disabled ? "not-allowed" : "text",
          fontFamily: typography.family.data.web,
          fontSize: text.size,
          minHeight: semanticTargets[density],
          opacity: disabled ? 0.62 : 1,
          // The amber border is the focus look; the browser would add its own ring outside it.
          outline: "none",
          paddingInline: semanticSpace.control,
          width: "100%",
        }}
      />
      {hasContent ? (
        <ComboboxPrimitive.Portal container={portalContainer as HTMLElement | ShadowRoot | null | undefined}>
          <ComboboxPrimitive.Positioner sideOffset={4} style={{ zIndex: 50 }}>
            <ComboboxPrimitive.Popup
              style={{
                backgroundColor: "var(--surfaceRaised)",
                border: "1px solid var(--borderDecorative)",
                borderRadius: semanticRadius.control,
                boxShadow: shadow,
                boxSizing: "border-box",
                color: "var(--contentPrimary)",
                maxHeight: "min(18rem, var(--available-height))",
                outline: "none",
                overflowY: "auto",
                padding: 6,
                width: "var(--anchor-width)",
              }}
            >
              <ComboboxPrimitive.List
                style={{ display: "flex", flexDirection: "column", gap: 2, opacity: stale ? 0.5 : 1 }}
              >
                {(option: ComboboxOption) => (
                  <ComboboxPrimitive.Item
                    disabled={option.disabled || stale}
                    key={option.value}
                    style={({ highlighted }) => ({
                      alignItems: "center",
                      backgroundColor: highlighted ? "var(--surfaceElevated)" : "transparent",
                      borderRadius: semanticRadius.control,
                      cursor: option.disabled || stale ? "default" : "pointer",
                      display: "flex",
                      gap: semanticSpace.control,
                      opacity: option.disabled ? 0.5 : 1,
                      outline: "none",
                      padding: "6px 8px",
                      userSelect: "none",
                    })}
                    value={option}
                  >
                    <span style={{ display: "flex", flex: 1, flexDirection: "column", minWidth: 0 }}>
                      <span
                        style={{
                          fontSize: text.size,
                          lineHeight: `${text.lineHeight}px`,
                          overflow: "hidden",
                          textOverflow: "ellipsis",
                          whiteSpace: "nowrap",
                        }}
                      >
                        {option.label}
                      </span>
                      {option.description ? (
                        <span
                          style={{
                            color: "var(--contentSecondary)",
                            fontSize: 12,
                            lineHeight: "16px",
                            overflow: "hidden",
                            textOverflow: "ellipsis",
                            whiteSpace: "nowrap",
                          }}
                        >
                          {option.description}
                        </span>
                      ) : null}
                    </span>
                    {option.trailing ? (
                      <span style={{ color: "var(--contentSecondary)", flexShrink: 0, fontSize: 12 }}>
                        {option.trailing}
                      </span>
                    ) : null}
                    {option.value === value ? (
                      <Icon decorative glyph={icons.selected} size="compact" tone="inherit" />
                    ) : null}
                  </ComboboxPrimitive.Item>
                )}
              </ComboboxPrimitive.List>
              {loading ? (
                <div role="status" style={statusStyle}>
                  Searching…
                </div>
              ) : null}
              {showEmpty ? (
                <div role="status" style={statusStyle}>
                  {emptyText}
                </div>
              ) : null}
              {errorText ? (
                <div role="alert" style={{ ...statusStyle, color: "var(--stateDanger)" }}>
                  {errorText}
                </div>
              ) : null}
            </ComboboxPrimitive.Popup>
          </ComboboxPrimitive.Positioner>
        </ComboboxPrimitive.Portal>
      ) : null}
    </ComboboxPrimitive.Root>
  );
};

export { Combobox };
