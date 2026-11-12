/// <reference lib="dom" />
import { Combobox as ComboboxPrimitive } from "@base-ui/react/combobox";
import { type ReactNode, useRef } from "react";

import { useFocusLook } from "../focus-look";
import { Icon, icons } from "../icon";
import type { Density } from "../tokens";
import { semanticRadius, semanticSpace, semanticTargets, typography } from "../tokens";
import type { ComboboxOption, ComboboxProps } from "./combobox.type";

const shadow = "0 10px 15px -3px rgb(0 0 0 / 0.1), 0 4px 6px -4px rgb(0 0 0 / 0.1)";
// The inline panel is a card on its own, over a dialog's scrim: the v2 mock's command palette.
const panelShadow = "0 24px 64px rgb(0 0 0 / 0.6)";

type InputKeyEvent = Parameters<NonNullable<ComboboxPrimitive.Input.Props["onKeyDown"]>>[0];

const statusStyle = {
  color: "var(--contentSecondary)",
  fontSize: 13,
  lineHeight: "18px",
  padding: `${semanticSpace.inline}px ${semanticSpace.control}px`,
} as const;

// Base UI's own Home and End move the text caret and nothing else, and it has no way to set the
// highlight from outside. The option's own pointer handler is the one door in: a mouse move onto an
// option highlights it exactly as hovering does, and the input keeps DOM focus.
const enabledOptions = (list: HTMLElement | null) =>
  Array.from(list?.querySelectorAll<HTMLElement>('[role="option"]:not([aria-disabled="true"])') ?? []);

const highlightOption = (option: HTMLElement | undefined) => {
  if (!option) return;
  option.dispatchEvent(new MouseEvent("mousemove", { bubbles: true, movementX: 1, movementY: 1 }));
  option.scrollIntoView?.({ block: "nearest" });
};

const OptionRow = ({
  density,
  option,
  stale,
  value,
}: {
  density: Density;
  option: ComboboxOption;
  stale: boolean;
  value: string | null;
}) => {
  const text = typography[density].body;
  return (
    <ComboboxPrimitive.Item
      disabled={option.disabled || stale}
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
        <span
          style={{
            alignItems: "center",
            color: "var(--contentSecondary)",
            display: "flex",
            flexShrink: 0,
            fontSize: 12,
            gap: semanticSpace.control,
          }}
        >
          {option.trailing}
        </span>
      ) : null}
      {option.value === value ? (
        <Icon decorative glyph={icons.selected} size="compact" tone="inherit" />
      ) : null}
    </ComboboxPrimitive.Item>
  );
};

// The web adapter, on Base UI Combobox: the combobox role, aria-activedescendant (DOM focus stays in
// the input while the highlight moves), arrow keys, Escape, and focus staying put when an option is
// pressed. Base UI draws nothing, so the field is Field's look value for value (the one amber border
// for focus, the data face) and the list is Menu's surface, read through the theme's CSS variables so
// light and dark both hold. The list portals, so a field inside a transformed row or a clipped card
// cannot cut it off.
//
// `inline` drops the popup: the list is always open under the field, in a panel that is the
// caller's to place (a command palette inside a dialog). Base UI's `inline` mode owns that
// contract; here it adds the header (adornments around a bare field), Home and End, wrapping at
// both ends, and the panel's surface.
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
  endAdornment,
  errorText,
  fill = false,
  groups,
  id,
  inline = false,
  inputValue,
  invalid = false,
  label,
  listLabel,
  loading = false,
  onEscape,
  onInputValueChange,
  onSelect,
  options,
  placeholder,
  portalContainer,
  stale = false,
  startAdornment,
  value = null,
}: ComboboxProps) => {
  const { focused, ...focusHandlers } = useFocusLook();
  const panelListRef = useRef<HTMLDivElement>(null);
  const text = typography[density].body;
  const drawnGroups = groups?.filter((group) => group.options.length > 0);
  const flat = drawnGroups ? drawnGroups.flatMap((group) => group.options) : (options ?? []);
  const selected = flat.find((option) => option.value === value) ?? null;
  const showEmpty = !loading && !errorText && flat.length === 0 && Boolean(emptyText);
  const hasContent = flat.length > 0 || loading || Boolean(errorText) || showEmpty;
  const items = drawnGroups
    ? drawnGroups.map((group) => ({ items: group.options, label: group.label, value: group.id }))
    : flat;

  const onKeyDown = (event: InputKeyEvent) => {
    if (event.ctrlKey || event.shiftKey || event.altKey || event.metaKey) return;
    if (event.key === "Escape") {
      // A floating list closes on its first Escape; only an Escape with nothing to close is the caller's.
      if (!onEscape || (!inline && event.currentTarget.getAttribute("aria-expanded") === "true")) return;
      event.preventBaseUIHandler();
      event.preventDefault();
      event.stopPropagation();
      onEscape();
      return;
    }
    if (!inline || flat.length === 0) return;
    const rows = enabledOptions(panelListRef.current);
    const active = event.currentTarget.getAttribute("aria-activedescendant");
    const first = rows[0];
    const last = rows[rows.length - 1];
    const target =
      event.key === "Home"
        ? first
        : event.key === "End"
          ? last
          : // Base UI steps from the last option back out to the field before it comes round; a
            // palette comes straight round.
            event.key === "ArrowDown" && last && active === last.id
            ? first
            : event.key === "ArrowUp" && first && active === first.id
              ? last
              : undefined;
    if (!target) return;
    event.preventBaseUIHandler();
    event.preventDefault();
    highlightOption(target);
  };

  const field = (
    <ComboboxPrimitive.Input
      {...focusHandlers}
      aria-describedby={describedBy}
      aria-invalid={invalid || undefined}
      aria-label={labelledBy ? undefined : label}
      aria-labelledby={labelledBy}
      autoComplete="off"
      id={id}
      onKeyDown={onKeyDown}
      placeholder={placeholder}
      style={
        inline
          ? {
              background: "transparent",
              border: "none",
              boxSizing: "border-box",
              color: "var(--contentPrimary)",
              flex: 1,
              fontFamily: typography.family.data.web,
              fontSize: text.size,
              minHeight: semanticTargets[density],
              minWidth: 0,
              // The panel's divider is the field's edge, as the mock draws it.
              outline: "none",
              padding: 0,
            }
          : {
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
            }
      }
    />
  );

  const statuses = (centered: boolean): ReactNode => {
    const style = centered
      ? { ...statusStyle, fontSize: text.size, padding: "24px 12px", textAlign: "center" as const }
      : statusStyle;
    return (
      <>
        {loading ? (
          <div role="status" style={style}>
            Searching…
          </div>
        ) : null}
        {showEmpty ? (
          <div role="status" style={style}>
            {emptyText}
          </div>
        ) : null}
        {errorText ? (
          <div role="alert" style={{ ...style, color: "var(--stateDanger)" }}>
            {errorText}
          </div>
        ) : null}
      </>
    );
  };

  const list =
    flat.length === 0 ? null : (
      <ComboboxPrimitive.List
        aria-label={inline ? listLabel : undefined}
        style={{ display: "flex", flexDirection: "column", gap: 2, opacity: stale ? 0.5 : 1 }}
      >
        {drawnGroups
          ? (group: (typeof items)[number] & { items: readonly ComboboxOption[] }) => (
              <ComboboxPrimitive.Group items={group.items} key={group.value}>
                <ComboboxPrimitive.GroupLabel
                  style={{
                    color: "var(--contentSecondary)",
                    fontFamily: typography.family.data.web,
                    fontSize: typography[density].metadata.size,
                    fontWeight: typography[density].metadata.weight,
                    lineHeight: `${typography[density].metadata.lineHeight}px`,
                    padding: "8px 8px 4px",
                  }}
                >
                  {"label" in group ? group.label : null}
                </ComboboxPrimitive.GroupLabel>
                <ComboboxPrimitive.Collection>
                  {(option: ComboboxOption) => (
                    <OptionRow
                      density={density}
                      key={option.value}
                      option={option}
                      stale={stale}
                      value={value}
                    />
                  )}
                </ComboboxPrimitive.Collection>
              </ComboboxPrimitive.Group>
            )
          : (option: ComboboxOption) => (
              <OptionRow density={density} key={option.value} option={option} stale={stale} value={value} />
            )}
      </ComboboxPrimitive.List>
    );

  return (
    <ComboboxPrimitive.Root<ComboboxOption>
      // "always" is Base UI's own mode (the first option stays highlighted whether or not the user
      // has typed); its Combobox typing only lists the boolean.
      autoHighlight={autoHighlight as boolean}
      disabled={disabled}
      filter={null}
      inline={inline}
      inputValue={inputValue}
      isItemEqualToValue={(item, chosen) => item.value === chosen.value}
      items={items}
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
      {...(inline ? { open: true } : {})}
      value={selected}
    >
      {inline ? (
        <div
          style={{
            backgroundColor: "var(--surfaceRaised)",
            border: fill ? "none" : "1px solid var(--borderDecorative)",
            borderRadius: fill ? 0 : semanticRadius.cardCompact,
            boxShadow: fill ? "none" : panelShadow,
            boxSizing: "border-box",
            color: "var(--contentPrimary)",
            display: "flex",
            flexDirection: "column",
            height: fill ? "100%" : undefined,
            overflow: "hidden",
            width: "100%",
          }}
        >
          <div
            style={{
              alignItems: "center",
              borderBottom: "1px solid var(--borderDecorative)",
              display: "flex",
              gap: semanticSpace.control,
              paddingInline: semanticSpace.control,
            }}
          >
            {startAdornment}
            {field}
            {endAdornment}
          </div>
          <div
            ref={panelListRef}
            style={{
              boxSizing: "border-box",
              flex: fill ? 1 : undefined,
              maxHeight: fill ? undefined : "20rem",
              overflowY: "auto",
              padding: 6,
            }}
          >
            {list}
            {statuses(true)}
          </div>
        </div>
      ) : (
        <>
          {field}
          {hasContent ? (
            <ComboboxPrimitive.Portal
              container={portalContainer as HTMLElement | ShadowRoot | null | undefined}
            >
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
                  {list}
                  {statuses(false)}
                </ComboboxPrimitive.Popup>
              </ComboboxPrimitive.Positioner>
            </ComboboxPrimitive.Portal>
          ) : null}
        </>
      )}
    </ComboboxPrimitive.Root>
  );
};

export { Combobox };
