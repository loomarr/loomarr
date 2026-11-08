import type {
  MenuActionProps,
  MenuContentProps,
  MenuGroupProps,
  MenuProps,
  MenuTriggerProps,
} from "./menu.type";

// No native surface opens a pointer menu yet, and a menu cannot degrade the way a tooltip does: its
// actions are reachable only through it. So the native adapter refuses to render rather than showing
// a trigger that opens nothing. A native caller wants a Dialog or a BottomSheet of actions.
const unsupported = (): never => {
  throw new Error("Menu is web-only; on native, offer the actions in a Dialog or BottomSheet");
};

const Menu = (_props: MenuProps) => unsupported();
const MenuTrigger = (_props: MenuTriggerProps) => unsupported();
const MenuContent = (_props: MenuContentProps) => unsupported();
const MenuAction = (_props: MenuActionProps) => unsupported();
const MenuGroup = (_props: MenuGroupProps) => unsupported();

export { Menu, MenuAction, MenuContent, MenuGroup, MenuTrigger };
