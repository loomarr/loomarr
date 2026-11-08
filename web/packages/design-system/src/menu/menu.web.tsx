/// <reference lib="dom" />
import { Menu as MenuPrimitive } from "@base-ui/react/menu";

import { semanticRadius } from "../tokens";
import type { MenuActionProps, MenuContentProps, MenuProps, MenuTriggerProps } from "./menu.type";

const shadow = "0 10px 15px -3px rgb(0 0 0 / 0.1), 0 4px 6px -4px rgb(0 0 0 / 0.1)";

// The web adapter, on Base UI Menu: roving focus, typeahead, Escape, focus back to the trigger, and
// the menu and menuitem roles. The panel is portalled, because a trigger inside a transformed row
// (the guide's virtualized rows) would otherwise scope the panel's stacking to that row. The surface
// reads the theme's CSS variables so light and dark both hold.
const Menu = ({ children, defaultOpen, onOpenChange }: MenuProps) => (
  <MenuPrimitive.Root
    defaultOpen={defaultOpen}
    {...(onOpenChange ? { onOpenChange: (open: boolean) => onOpenChange(open) } : {})}
  >
    {children}
  </MenuPrimitive.Root>
);

const MenuTrigger = ({ children }: MenuTriggerProps) => <MenuPrimitive.Trigger render={children} />;

const MenuContent = ({ align, children, portalContainer, side, width }: MenuContentProps) => (
  <MenuPrimitive.Portal container={portalContainer as HTMLElement | ShadowRoot | null | undefined}>
    <MenuPrimitive.Positioner align={align} side={side} sideOffset={6} style={{ zIndex: 50 }}>
      <MenuPrimitive.Popup
        style={{
          backgroundColor: "var(--surfaceRaised)",
          border: "1px solid var(--borderDecorative)",
          borderRadius: semanticRadius.control,
          boxShadow: shadow,
          color: "var(--contentPrimary)",
          display: "flex",
          flexDirection: "column",
          gap: 2,
          minWidth: 160,
          outline: "none",
          padding: 6,
          width,
        }}
      >
        {children}
      </MenuPrimitive.Popup>
    </MenuPrimitive.Positioner>
  </MenuPrimitive.Portal>
);

const MenuAction = ({ children, closeOnSelect = true, disabled, icon, onSelect, tone }: MenuActionProps) => (
  <MenuPrimitive.Item
    closeOnClick={closeOnSelect}
    disabled={disabled}
    onClick={onSelect}
    style={({ highlighted }) => ({
      alignItems: "center",
      backgroundColor: highlighted
        ? tone === "danger"
          ? "var(--stateDangerSurface)"
          : "var(--surfaceElevated)"
        : "transparent",
      borderRadius: semanticRadius.control,
      color: tone === "danger" ? "var(--stateDanger)" : "var(--contentPrimary)",
      cursor: disabled ? "default" : "pointer",
      display: "flex",
      fontSize: 14,
      gap: 8,
      lineHeight: "20px",
      opacity: disabled ? 0.5 : 1,
      outline: "none",
      padding: "6px 8px",
      userSelect: "none",
    })}
  >
    {icon}
    {children}
  </MenuPrimitive.Item>
);

export { Menu, MenuAction, MenuContent, MenuTrigger };
