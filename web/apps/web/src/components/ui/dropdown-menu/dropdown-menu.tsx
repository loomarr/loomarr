import { Menu as MenuPrimitive } from "@base-ui/react/menu";
import { Separator as SeparatorPrimitive } from "@base-ui/react/separator";
import { Check } from "lucide-react";
import { cn } from "@/lib/utils";
import type {
  DropdownMenuCheckboxItemProps,
  DropdownMenuContentProps,
  DropdownMenuItemProps,
  DropdownMenuLabelProps,
  DropdownMenuSeparatorProps,
} from "./dropdown-menu.type";

// DropdownMenu — the app's menu primitive (§14), a thin themed wrapper over Base UI Menu, in the
// same shape as `select`/`dialog` (a `const` per part, `cn` from `@/lib`). A MENU — a trigger opens
// a list of choices — distinct from a form `<select>`; the primitive gives it roving focus,
// typeahead, Escape-to-close and correct ARIA.
//
// Deliberately SMALL: only the parts in use (Trigger, Content, Group, Label, CheckboxItem,
// Separator). Base UI ships sub-menus, radio groups, link items and a viewport too — none are used,
// so they are left out rather than carried as dead surface. Add a part back when something needs it.
const DropdownMenu = MenuPrimitive.Root;
const DropdownMenuTrigger = MenuPrimitive.Trigger;
const DropdownMenuGroup = MenuPrimitive.Group;

// Content — the floating panel, portalled so it escapes an `overflow:hidden` ancestor (the player's
// control bar clips its own overflow).
//
// ⚠ The old Radix version carried ~10 `data-[state=…]` / `data-[side=…]` animation classes
// (`animate-in`, `zoom-in-95`, `slide-in-from-top-1`, …). They are GONE rather than translated,
// because they never did anything: `tailwindcss-animate` is not a dependency here (see the note in
// styles.css), so every one of those class names compiled to nothing. Deleting them is a no-op on
// screen — which is exactly why the visual baselines must not move for this file.
const DropdownMenuContent = ({
  className,
  children,
  side,
  align,
  sideOffset = 6,
  alignOffset,
  ...props
}: DropdownMenuContentProps) => (
  <MenuPrimitive.Portal>
    <MenuPrimitive.Positioner
      side={side}
      align={align}
      sideOffset={sideOffset}
      alignOffset={alignOffset}
      className="z-50"
    >
      <MenuPrimitive.Popup
        className={cn(
          "min-w-40 overflow-hidden rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-lg",
          className,
        )}
        {...props}
      >
        {children}
      </MenuPrimitive.Popup>
    </MenuPrimitive.Positioner>
  </MenuPrimitive.Portal>
);

// Item — a plain action row. Added in V50b for the channels-list ⋮ menu, per this file's own
// "add a part back when something needs it" rule; the player's track menu still uses CheckboxItem.
const DropdownMenuItem = ({ className, ...props }: DropdownMenuItemProps) => (
  <MenuPrimitive.Item
    className={cn(
      "flex cursor-pointer select-none items-center gap-2 rounded px-2 py-1.5 text-left text-sm outline-none transition-colors",
      "data-[disabled]:pointer-events-none data-[highlighted]:bg-static-700 data-[highlighted]:text-static-0 data-[disabled]:opacity-50",
      className,
    )}
    {...props}
  />
);

// Label — a non-interactive heading (mono/uppercase caption, the app's data-label idiom).
//
// ⚠ Base UI has no standalone Label the way Radix did; a label belongs to a GROUP, and naming the
// group is the point — `<DropdownMenuGroup>` + this together give the item list an accessible name
// instead of a floating heading that only sighted users associate with the rows beneath it.
const DropdownMenuLabel = ({ className, ...props }: DropdownMenuLabelProps) => (
  <MenuPrimitive.GroupLabel
    className={cn("px-2 py-1.5 font-mono text-2xs text-muted-foreground uppercase tracking-wide", className)}
    {...props}
  />
);

// CheckboxItem — a selectable row that shows a check when active. Exactly-one-selected (the current
// audio track / subtitle mode) reads fine as a set of checkbox items, and the primitive owns the
// checked state.
//
// ⚠ `closeOnClick` defaults to TRUE here, inverting Base UI's own default. Radix closed the menu on
// pick and the player's track menu was built around that; Base UI keeps checkbox menus open (right
// for a multi-select filter, wrong for "which audio track"). Preserved as the app's default, still
// overridable per item.
const DropdownMenuCheckboxItem = ({
  className,
  children,
  checked,
  closeOnClick = true,
  ...props
}: DropdownMenuCheckboxItemProps) => (
  <MenuPrimitive.CheckboxItem
    checked={checked}
    closeOnClick={closeOnClick}
    className={cn(
      "relative flex cursor-pointer select-none items-center rounded-sm py-1.5 pr-2 pl-7 text-sm outline-none",
      "data-[disabled]:pointer-events-none data-[highlighted]:bg-accent data-[highlighted]:text-accent-foreground data-[disabled]:opacity-50",
      className,
    )}
    {...props}
  >
    <span className="absolute left-2 flex size-3.5 items-center justify-center">
      <MenuPrimitive.CheckboxItemIndicator>
        <Check className="size-3.5" aria-hidden />
      </MenuPrimitive.CheckboxItemIndicator>
    </span>
    {children}
  </MenuPrimitive.CheckboxItem>
);

// ⚠ Separator comes from Base UI's top-level component, not from Menu — unlike Radix, `Menu` ships
// no Separator part of its own.
const DropdownMenuSeparator = ({ className, ...props }: DropdownMenuSeparatorProps) => (
  <SeparatorPrimitive className={cn("-mx-1 my-1 h-px bg-border", className)} {...props} />
);

export {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
};
