import { View } from "@tamagui/core";
import type { ComponentProps, ReactNode } from "react";

import type { semanticSpace } from "../tokens";

type StackGap = keyof typeof semanticSpace | "none";
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

type StackProps = Omit<
  ComponentProps<typeof View>,
  "alignItems" | "children" | "columnGap" | "flexDirection" | "flexWrap" | "gap" | "justifyContent" | "rowGap"
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
