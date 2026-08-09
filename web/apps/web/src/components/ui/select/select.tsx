import { Select as SelectPrimitive } from "@base-ui/react/select";
import { Check, ChevronDown } from "lucide-react";
import { Children, isValidElement, useMemo } from "react";
import { cn } from "@/lib";
import type { SelectContentProps, SelectItemProps, SelectRootProps, SelectTriggerProps } from "./select.type";

// The enum control (config-design §2 KindEnum), on Base UI Select (design §14). A native
// <select> shipped first — accessible and zero-dep — but its OS option list is unstyleable
// (off-theme popups), so this is a themed listbox: the trigger matches Input (same
// border-input boundary + signal focus ring, so a form reads as one system, §2.3) and the
// popover is a `popover`-token surface with hover + selected states.
const SelectGroup = SelectPrimitive.Group;
const SelectValue = SelectPrimitive.Value;

const SelectTrigger = ({ className, children, ...props }: SelectTriggerProps) => (
  <SelectPrimitive.Trigger
    className={cn(
      "flex h-9 w-full cursor-pointer items-center justify-between gap-2 rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:cursor-not-allowed disabled:opacity-50 data-[placeholder]:text-muted-foreground [&>span]:truncate",
      className,
    )}
    {...props}
  >
    {children}
    <SelectPrimitive.Icon render={<ChevronDown className="size-4 shrink-0 text-muted-foreground" />} />
  </SelectPrimitive.Trigger>
);

const SelectContent = ({
  className,
  children,
  side,
  align,
  sideOffset,
  alignOffset,
  // Base UI's replacement for Radix's `position="popper"`. It defaults to TRUE, which overlaps
  // the popup on the trigger so the selected row covers the trigger text; the app has always
  // dropped the list BELOW the trigger, so the wrapper flips the default back.
  alignItemWithTrigger = false,
  ...props
}: SelectContentProps) => (
  <SelectPrimitive.Portal>
    <SelectPrimitive.Positioner
      side={side}
      align={align}
      sideOffset={sideOffset}
      alignOffset={alignOffset}
      alignItemWithTrigger={alignItemWithTrigger}
      className="z-50"
    >
      <SelectPrimitive.Popup
        className={cn(
          // `--anchor-width` is Base UI's `--radix-select-trigger-width`: the list is never
          // narrower than the trigger it drops from. `--available-height` keeps a long list
          // inside the viewport instead of running off the bottom.
          "relative max-h-[min(24rem,var(--available-height))] min-w-[max(8rem,var(--anchor-width))] overflow-hidden rounded-md border border-border bg-popover text-popover-foreground shadow-md",
          // Fade + rise on open — off under reduced-motion (and frozen in the visual suite).
          "motion-safe:animate-select-in",
          className,
        )}
        {...props}
      >
        <SelectPrimitive.List className="p-1">{children}</SelectPrimitive.List>
      </SelectPrimitive.Popup>
    </SelectPrimitive.Positioner>
  </SelectPrimitive.Portal>
);

const SelectItem = ({ className, children, ...props }: SelectItemProps) => (
  <SelectPrimitive.Item
    className={cn(
      "relative flex w-full cursor-pointer select-none items-center rounded-sm py-1.5 pr-8 pl-2 text-sm outline-none data-[disabled]:pointer-events-none data-[highlighted]:bg-accent data-[highlighted]:text-accent-foreground data-[disabled]:opacity-50",
      className,
    )}
    {...props}
  >
    <span className="absolute right-2 flex size-3.5 items-center justify-center">
      <SelectPrimitive.ItemIndicator>
        <Check className="size-4 text-signal" />
      </SelectPrimitive.ItemIndicator>
    </span>
    <SelectPrimitive.ItemText>{children}</SelectPrimitive.ItemText>
  </SelectPrimitive.Item>
);

// ⚠ WHY THIS WALK EXISTS — do not replace it with `items` at the call sites without reading this.
//
// Base UI renders `<SelectValue>` as the selected item's LABEL only when `<Select.Root>` is given
// an `items` map; with no `items` it renders the raw VALUE. The app's 23 selects all pass their
// options as inline `<SelectItem>` JSX, so the swap from Radix (which read the label off the
// selected `ItemText`) would have turned every trigger into a raw enum token — "240" where the
// list says "4 hours", "openai" where it says "OpenAI". Nothing type-checks or axe-checks that.
//
// The obvious fix — hand-write `items={[…]}` beside each list — duplicates every label
// expression, and several are computed (`dayLabel(d, mountedAt)`). That is a second list that
// must agree with the first, which is the drift this codebase has already been bitten by more
// than once. So the labels are DERIVED from the one place they are written: the items themselves.
//
// The walk is deliberately shallow in concept — it recurses through fragments, arrays and
// conditionals to find `SelectItem` elements — and it only supplies a default: an explicit
// `items` prop still wins, for a caller that needs object values or a label the JSX doesn't hold.
const collectItems = (node: React.ReactNode, out: { label: React.ReactNode; value: unknown }[]): void => {
  Children.forEach(node, (child) => {
    if (!isValidElement(child)) return;
    if (child.type === SelectItem) {
      const { value, children } = child.props as SelectItemProps;
      out.push({ label: children, value });
      return;
    }
    const nested = (child.props as { children?: React.ReactNode }).children;
    if (nested) collectItems(nested, out);
  });
};

const Select = ({ children, items, onValueChange, ...props }: SelectRootProps) => {
  const derived = useMemo(() => {
    if (items) return items;
    const found: { label: React.ReactNode; value: unknown }[] = [];
    collectItems(children, found);
    return found.length > 0 ? found : undefined;
  }, [children, items]);

  return (
    <SelectPrimitive.Root
      items={derived}
      // See select.type.ts: `null` is unreachable here because no select in this app renders a
      // clearable (`{ value: null }`) item, so the callback is narrowed rather than every caller.
      onValueChange={onValueChange ? (value) => value !== null && onValueChange(value) : undefined}
      {...props}
    >
      {children}
    </SelectPrimitive.Root>
  );
};

export { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue };
