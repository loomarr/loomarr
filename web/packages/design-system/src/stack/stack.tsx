import { isWeb, View } from "@tamagui/core";
import type { ComponentProps, FormHTMLAttributes, ReactNode } from "react";
import { useSyncExternalStore } from "react";
import { Dimensions } from "react-native";

import { breakpoints, type semanticSpace, type spaceSteps } from "../tokens";

/**
 * A semantic space (`control`, `inline` …) or a step of the spacing scale, keyed as Tailwind keys it
 * (`"4"` is 16px, `"1.5"` is 6px), so a hand-rolled `gap-*` moves over at the same size.
 */
type StackGap = keyof typeof semanticSpace | `${keyof typeof spaceSteps}` | "none";
type StackAlign = "start" | "center" | "end" | "stretch" | "baseline";
type StackJustify = "start" | "center" | "end" | "between";
type StackBreakpoint = keyof typeof breakpoints;

/**
 * A value that can change with the window's width: a plain value holds at every width, or
 * `{ base, sm, md, lg }` gives the narrow value and the one that takes over from each breakpoint up
 * (Tailwind's `flex-col sm:flex-row` is `{ base: "column", sm: "row" }`). A breakpoint left out
 * keeps the value of the narrower one.
 */
type StackResponsive<Value> = Value | ({ base: Value } & { [Breakpoint in StackBreakpoint]?: Value });

const alignItems = {
  baseline: "baseline",
  center: "center",
  end: "flex-end",
  start: "flex-start",
  stretch: "stretch",
} as const;

const justifyContent = {
  between: "space-between",
  center: "center",
  end: "flex-end",
  start: "flex-start",
} as const;

/**
 * The attributes of a `<form>` a Stack can carry. Web only: on native there is no form element, so
 * they are ignored there. Only a Stack with `render="form"` may set them.
 */
type StackFormAttributes = Pick<
  FormHTMLAttributes<HTMLFormElement>,
  | "acceptCharset"
  | "action"
  | "autoComplete"
  | "encType"
  | "method"
  | "name"
  | "noValidate"
  | "onSubmit"
  | "target"
>;

type StackRender = ComponentProps<typeof View>["render"];

type StackOwnProps = Omit<
  ComponentProps<typeof View>,
  | "alignItems"
  | "children"
  | "columnGap"
  | "flexDirection"
  | "flexWrap"
  | "gap"
  | "justifyContent"
  | "render"
  | "rowGap"
> & {
  /** Cross-axis alignment. Left out, children stretch (a column fills its width). */
  align?: StackResponsive<StackAlign>;
  children?: ReactNode;
  /** The main axis. `column` (the default) stacks top to bottom; `row` lays out left to right. */
  direction?: StackResponsive<"column" | "row">;
  /** Space between children, from the spacing tokens. Left out, there is none. */
  gap?: StackResponsive<StackGap>;
  /**
   * Space between the lines of a wrapping row, from the same tokens, overriding `gap` on that
   * axis (Tailwind's `gap-y-*`). Left out, the line spacing is `gap`.
   */
  rowGap?: StackResponsive<StackGap>;
  /**
   * Space between the children along a row, from the same tokens, overriding `gap` on that axis
   * (Tailwind's `gap-x-*`). Left out, the spacing is `gap`.
   */
  columnGap?: StackResponsive<StackGap>;
  /** Main-axis distribution. Left out, children pack at the start. */
  justify?: StackResponsive<StackJustify>;
  /** Let a row break onto further lines instead of overflowing. The gap also separates the lines. */
  wrap?: boolean;
};

type Active = "base" | StackBreakpoint;
const breakpointNames = Object.keys(breakpoints) as StackBreakpoint[];

const minWidth = (name: StackBreakpoint) => `(min-width: ${breakpoints[name]}px)`;

// On the web the breakpoints are the browser's own min-width media queries, so a layout flips at
// exactly the width Tailwind's `sm:` does (the window's width counts its scrollbar, which React
// Native's Dimensions leaves out). Native has no media queries: it reads the window's width.
type MediaQuery = {
  addEventListener: (type: "change", listener: () => void) => void;
  matches: boolean;
  removeEventListener: (type: "change", listener: () => void) => void;
};

// Read off the global rather than named as `window`: the native packages typecheck this file
// without the DOM types, and there the answer is always "no media queries".
const matchMedia = (): ((query: string) => MediaQuery) | undefined =>
  isWeb
    ? (globalThis as { matchMedia?: (query: string) => MediaQuery }).matchMedia?.bind(globalThis)
    : undefined;

const readBreakpoint = (): Active => {
  const query = matchMedia();
  return breakpointNames.reduce<Active>((active, name) => {
    const reached = query
      ? query(minWidth(name)).matches
      : Dimensions.get("window").width >= breakpoints[name];
    return reached ? name : active;
  }, "base");
};

const subscribeToWidth = (onChange: () => void) => {
  const query = matchMedia();
  if (query) {
    const queries = breakpointNames.map((name) => query(minWidth(name)));
    for (const list of queries) list.addEventListener("change", onChange);
    return () => {
      for (const list of queries) list.removeEventListener("change", onChange);
    };
  }
  const subscription = Dimensions.addEventListener("change", onChange);
  return () => subscription.remove();
};
const noSubscription = () => () => undefined;

/**
 * The widest breakpoint the window has reached, `base` below the first. It re-renders its caller
 * only when that changes, not on every pixel of a resize, and reads the real width on the first
 * render so a narrow window never paints the wide layout. A Stack with no responsive value passes
 * `false` and neither listens nor re-renders.
 */
const useActiveBreakpoint = (enabled: boolean): Active =>
  useSyncExternalStore(
    enabled ? subscribeToWidth : noSubscription,
    () => (enabled ? readBreakpoint() : "base"),
    () => "base",
  );

const isResponsive = <Value,>(
  value: StackResponsive<Value> | undefined,
): value is { base: Value } & { [Breakpoint in StackBreakpoint]?: Value } =>
  typeof value === "object" && value !== null && "base" in value;

/** The value in force at `active`: the base, then each breakpoint reached, the wider one winning. */
const resolve = <Value,>(value: StackResponsive<Value>, active: Active): Value => {
  if (!isResponsive(value)) return value;
  let resolved = value.base;
  for (const name of breakpointNames) {
    if (breakpoints[name] > (active === "base" ? 0 : breakpoints[active])) break;
    resolved = value[name] ?? resolved;
  }
  return resolved;
};

/**
 * `render="form"` turns the Stack into a `<form>` and unlocks its attributes (`noValidate`,
 * `onSubmit` …); any other element takes none of them, so a stray one is a type error rather than a
 * silently dropped prop.
 */
type StackProps = StackOwnProps &
  (
    | ({ render: "form" } & StackFormAttributes)
    | ({ render?: StackRender } & { [Attribute in keyof StackFormAttributes]?: never })
  );

/**
 * Layout only: children in a row or a column, spaced by the spacing tokens. It draws nothing of its
 * own (no colour, border or padding), so a call site keeps what it is for in its own element and
 * hands the arrangement here. Web and native share one implementation (Tamagui's View maps to a
 * flex <div> on web and a View on native), so a row reads the same on both.
 *
 * A Stack never shrinks unless asked (`flexShrink` is 0, as in React Native): a Stack that shrank by
 * default collapsed below its content in a height-capped column. One that must give way, usually a
 * truncating cell in a row, sets `flexShrink={1}` and `minWidth={0}`; a `flexBasis="0%" flexGrow={1}`
 * child sets `flexShrink={1}` too.
 */
const Stack = ({
  align = "stretch",
  direction = "column",
  columnGap,
  gap = "none",
  justify = "start",
  rowGap,
  wrap = false,
  ...props
}: StackProps) => {
  const active = useActiveBreakpoint(
    [align, direction, gap, justify, rowGap, columnGap].some((value) => isResponsive<string>(value)),
  );
  const space = (value: StackGap) => (value === "none" ? 0 : value);
  const resolvedGap = resolve(gap, active);
  const resolvedRowGap = rowGap === undefined ? undefined : resolve(rowGap, active);
  const resolvedColumnGap = columnGap === undefined ? undefined : resolve(columnGap, active);
  // One `gap` when no axis is named. Once one is, both axes are written out, so a per-axis value
  // never depends on how the shorthand and the longhand are ordered in the stylesheet.
  const spacing =
    resolvedRowGap === undefined && resolvedColumnGap === undefined
      ? { gap: space(resolvedGap) }
      : {
          columnGap: space(resolvedColumnGap ?? resolvedGap),
          rowGap: space(resolvedRowGap ?? resolvedGap),
        };
  return (
    // A Stack does not shrink as a flex item (flex-shrink 0), as in React Native: in a height-capped
    // column a shrinking Stack collapses below its content. A Stack that must give way, usually a
    // truncating cell in a row, sets `flexShrink={1}` and `minWidth={0}` itself.
    <View
      flexShrink={0}
      {...props}
      alignItems={alignItems[resolve(align, active)]}
      flexDirection={resolve(direction, active)}
      flexWrap={wrap ? "wrap" : "nowrap"}
      justifyContent={justifyContent[resolve(justify, active)]}
      {...spacing}
    />
  );
};

export type { StackAlign, StackBreakpoint, StackGap, StackJustify, StackProps, StackResponsive };
export { Stack };
