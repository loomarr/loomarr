import type { Select as SelectPrimitive } from "@base-ui/react/select";

// SelectContent collapses four Base UI parts (Portal → Positioner → Popup → List) into the one
// component the app writes, so the positioning props are surfaced deliberately: they belong to
// the POSITIONER, the surface styling to the POPUP. Radix took both on a single `Content`.
//
// `alignItemWithTrigger` is Base UI's replacement for Radix's `position="popper" | "item-aligned"`.
// It defaults to TRUE (the popup overlaps the trigger so the selected row sits over the trigger
// text); the app wants the popper behaviour it had under Radix, so the wrapper defaults it false.
type SelectContentProps = SelectPrimitive.Popup.Props &
  Pick<
    SelectPrimitive.Positioner.Props,
    "side" | "align" | "sideOffset" | "alignOffset" | "alignItemWithTrigger"
  >;

type SelectTriggerProps = SelectPrimitive.Trigger.Props;
type SelectItemProps = SelectPrimitive.Item.Props;

// The Root wrapper keeps Base UI's own props verbatim; `items` is merely made explicit because
// the wrapper DERIVES it from the `<SelectItem>` children when a caller omits it (see the walk in
// select.tsx). An explicit `items` always wins.
//
// ⚠ PINNED TO `string`, deliberately. Base UI's Root is generic over the item value, and leaving
// it unparameterised makes `Value` infer as `any` — which silently turned all 23
// `onValueChange={(v) => …}` handlers into implicit-`any` params (Radix's callback was typed
// `(value: string)`). Every select in this app is a string-valued enum, so pinning restores the
// typing the call sites already relied on. A future object-valued select widens this on purpose,
// with the call sites' types changing as a visible consequence rather than silently.
//
// `onValueChange` is also NARROWED to non-null. Base UI can emit `null` because a select may
// carry a `{ value: null }` item to mean "clear"; Radix had no such case and neither does this
// app — no select here renders a null item, so the callback cannot fire with one. Widening all 23
// handlers to accept a `null` that cannot occur would be noise at every call site, so the wrapper
// absorbs it. ⚠ If a clearable select is ever added, this narrowing is the thing to remove FIRST —
// otherwise the clear silently does nothing.
type SelectRootProps = Omit<SelectPrimitive.Root.Props<string>, "onValueChange"> & {
  onValueChange?: (value: string) => void;
};

export type { SelectContentProps, SelectItemProps, SelectRootProps, SelectTriggerProps };
