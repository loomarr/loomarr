import { View } from "@tamagui/core";
import type { ComponentProps, FormHTMLAttributes, ReactNode } from "react";

import type { semanticSpace, spaceSteps } from "../tokens";

/**
 * A semantic space (`control`, `inline` …) or a step of the spacing scale, keyed as Tailwind keys it
 * (`"4"` is 16px, `"1.5"` is 6px), so a hand-rolled `gap-*` moves over at the same size.
 */
type StackGap = keyof typeof semanticSpace | `${keyof typeof spaceSteps}` | "none";
type StackAlign = "start" | "center" | "end" | "stretch" | "baseline";
type StackJustify = "start" | "center" | "end" | "between";

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
  align?: StackAlign;
  children?: ReactNode;
  /** The main axis. `column` (the default) stacks top to bottom; `row` lays out left to right. */
  direction?: "column" | "row";
  /** Space between children, from the spacing tokens. Left out, there is none. */
  gap?: StackGap;
  /** Main-axis distribution. Left out, children pack at the start. */
  justify?: StackJustify;
  /** Let a row break onto further lines instead of overflowing. The gap also separates the lines. */
  wrap?: boolean;
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
 */
const Stack = ({
  align = "stretch",
  direction = "column",
  gap = "none",
  justify = "start",
  wrap = false,
  ...props
}: StackProps) => (
  // Tamagui's View never shrinks as a flex item (flex-shrink 0), unlike the <div> a call site moves
  // from, so a Stack in a flex row would overflow rather than give way. A caller can still set it.
  <View
    flexShrink={1}
    {...props}
    alignItems={alignItems[align]}
    flexDirection={direction}
    flexWrap={wrap ? "wrap" : "nowrap"}
    gap={gap === "none" ? 0 : gap}
    justifyContent={justifyContent[justify]}
  />
);

export type { StackAlign, StackGap, StackJustify, StackProps };
export { Stack };
