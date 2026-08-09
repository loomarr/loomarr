import type { Menu as MenuPrimitive } from "@base-ui/react/menu";
import type { Separator as SeparatorPrimitive } from "@base-ui/react/separator";

// Content collapses three Base UI parts (Portal → Positioner → Popup) into the one component the
// app writes: `side`/`align`/`sideOffset` belong to the POSITIONER, the surface styling to the
// POPUP. Radix took both on a single `Content`.
type DropdownMenuContentProps = MenuPrimitive.Popup.Props &
  Pick<MenuPrimitive.Positioner.Props, "side" | "align" | "sideOffset" | "alignOffset">;

type DropdownMenuCheckboxItemProps = MenuPrimitive.CheckboxItem.Props;
type DropdownMenuItemProps = MenuPrimitive.Item.Props;
type DropdownMenuLabelProps = MenuPrimitive.GroupLabel.Props;
type DropdownMenuSeparatorProps = SeparatorPrimitive.Props;

export type {
  DropdownMenuCheckboxItemProps,
  DropdownMenuContentProps,
  DropdownMenuItemProps,
  DropdownMenuLabelProps,
  DropdownMenuSeparatorProps,
};
