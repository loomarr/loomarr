import {
  Hint,
  type HintProps,
  MenuList,
  type MenuListProps,
  SelectControl,
  type SelectControlProps,
  Tabs,
  type TabsProps,
} from "@loomarr/design-system";
import type { ReactElement, KeyboardEvent as ReactKeyboardEvent } from "react";
import { useEffect, useRef, useState } from "react";

const enabledItems = (root: HTMLElement, role: "menuitem" | "tab") =>
  Array.from(root.querySelectorAll<HTMLElement>(`[role="${role}"]:not([aria-disabled="true"])`));

const moveFocus = (event: ReactKeyboardEvent<HTMLElement>, role: "menuitem" | "tab", activate: boolean) => {
  const items = enabledItems(event.currentTarget, role);
  if (items.length === 0) return;
  const current = items.indexOf(document.activeElement as HTMLElement);
  let next: number | undefined;
  if (event.key === "Home") next = 0;
  if (event.key === "End") next = items.length - 1;
  if (event.key === "ArrowRight" || event.key === "ArrowDown") next = (current + 1) % items.length;
  if (event.key === "ArrowLeft" || event.key === "ArrowUp") {
    next = (current - 1 + items.length) % items.length;
  }
  if (next === undefined) return;
  event.preventDefault();
  items[next]?.focus();
  if (activate) items[next]?.click();
};

const BrowserTabs = <Value extends string>(props: TabsProps<Value>) => (
  <fieldset
    onKeyDown={(event) => moveFocus(event, "tab", true)}
    style={{ border: 0, margin: 0, minWidth: 0, padding: 0 }}
  >
    <Tabs {...props} />
  </fieldset>
);

type BrowserMenuListProps<Value extends string> = MenuListProps<Value> & { onDismiss?: () => void };

const BrowserMenuList = <Value extends string>({ onDismiss, ...props }: BrowserMenuListProps<Value>) => (
  <fieldset
    onKeyDown={(event) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onDismiss?.();
        return;
      }
      moveFocus(event, "menuitem", false);
    }}
    style={{ border: 0, margin: 0, minWidth: 0, padding: 0 }}
  >
    <MenuList {...props} />
  </fieldset>
);

type BrowserSelectControlProps<Value extends string> = Omit<
  SelectControlProps<Value>,
  "onOpenChange" | "open"
> & { initialOpen?: boolean };

const BrowserSelectControl = <Value extends string>({
  initialOpen = false,
  ...props
}: BrowserSelectControlProps<Value>) => {
  const [open, setOpen] = useState(initialOpen);
  const root = useRef<HTMLFieldSetElement>(null);

  useEffect(() => {
    if (!open) return;
    root.current?.querySelector<HTMLElement>(`[role="radio"][aria-checked="true"]`)?.focus();
  }, [open]);

  return (
    <fieldset
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false);
      }}
      onKeyDown={(event) => {
        if (event.key !== "Escape" || !open) return;
        event.preventDefault();
        setOpen(false);
        root.current?.querySelector<HTMLElement>("[aria-expanded]")?.focus();
      }}
      ref={root}
      style={{ border: 0, margin: 0, minWidth: 0, padding: 0 }}
    >
      <SelectControl
        {...props}
        onOpenChange={setOpen}
        onValueChange={(value) => {
          props.onValueChange(value);
          requestAnimationFrame(() => root.current?.querySelector<HTMLElement>("[aria-expanded]")?.focus());
        }}
        open={open}
      />
    </fieldset>
  );
};

type BrowserHintProps = Omit<HintProps, "children" | "visible"> & { children: ReactElement };

const BrowserHint = ({ children, ...props }: BrowserHintProps) => {
  const [focused, setFocused] = useState(false);
  const [hovered, setHovered] = useState(false);
  return (
    <fieldset
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setFocused(false);
      }}
      onFocus={() => setFocused(true)}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      style={{ border: 0, margin: 0, minWidth: 0, padding: 0 }}
    >
      <Hint {...props} visible={focused || hovered}>
        {children}
      </Hint>
    </fieldset>
  );
};

export type { BrowserHintProps, BrowserMenuListProps, BrowserSelectControlProps };
export { BrowserHint, BrowserMenuList, BrowserSelectControl, BrowserTabs };
