import { isWeb, View } from "@tamagui/core";
import type { ComponentProps, CSSProperties, ReactElement, ReactNode } from "react";
import { Children, createContext, isValidElement, useContext } from "react";

import {
  alignItems,
  isResponsive,
  resolve,
  type StackAlign,
  type StackGap,
  type StackResponsive,
  useActiveBreakpoint,
} from "../stack/stack";

/** How many columns a grid item covers: a count, or `full` for every column of the grid. */
type GridSpan = number | "full";

/**
 * One column's width in a track list, written as the CSS track it draws: `auto` (as wide as its
 * content), a share of the leftover width (`1fr`, `1.5fr`), a fixed size (`110px`, `7rem`), or a
 * `minmax(…)` range (`minmax(12rem,1fr)`). A bare `1fr` can grow past its share to fit wide content,
 * as Tailwind's `grid-cols-[auto_1fr]` does; write `minmax(0,1fr)` for a share that never does.
 */
type GridTrack = "auto" | `${number}fr` | `${number}px` | `${number}rem` | `minmax(${string})`;

/** A count of equal columns, or a list of tracks for columns of different widths. */
type GridColumnSpec = number | readonly GridTrack[];

type GridOwnProps = Omit<
  ComponentProps<typeof View>,
  "alignItems" | "children" | "columnGap" | "columns" | "direction" | "gap" | "render" | "rowGap"
> & {
  /** Cross-axis alignment of each item within its row. Left out, items stretch. */
  align?: StackResponsive<StackAlign>;
  children?: ReactNode;
  /**
   * How many equal columns, or a list of tracks for columns of different widths, which can change
   * with the window's width: `{ base: 1, sm: 2 }` is Tailwind's `grid-cols-1 sm:grid-cols-2`, and
   * `{ base: 1, md: ["17rem", "1fr"] }` is `grid-cols-1 md:grid-cols-[17rem_1fr]`. Left out, one.
   */
  columns?: StackResponsive<GridColumnSpec>;
  /** Space between the columns, from the spacing tokens, overriding `gap` on that axis (`gap-x-*`). */
  columnGap?: StackResponsive<StackGap>;
  /** Space between the cells in both directions, from the spacing tokens. Left out, there is none. */
  gap?: StackResponsive<StackGap>;
  /** The element drawn on web (`ul`, `dl` …); native has no elements and ignores it. */
  render?: ComponentProps<typeof View>["render"];
  /** Space between the rows, from the spacing tokens, overriding `gap` on that axis (`gap-y-*`). */
  rowGap?: StackResponsive<StackGap>;
};

type GridProps = GridOwnProps;

type GridItemProps = ComponentProps<typeof View> & {
  /** Columns this item covers, which can change with the window's width (`sm:col-span-2`). Left out, one. */
  span?: StackResponsive<GridSpan>;
  /**
   * The column the item starts in, counting from 1, which can change with the window's width
   * (`col-[2/-1]` is `from={2}` with `span="full"`, which then runs to the last column; a grid with
   * one column needs `from={{ base: 1, sm: 2 }}`, since column 2 does not exist there). Left out, the
   * item takes the next free cell. Web only: native packs items in order and ignores it.
   */
  from?: StackResponsive<number>;
};

// The column count in force, so an item can read `full` as "every column".
const GridColumns = createContext(1);

const space = (value: StackGap) => (value === "none" ? 0 : value);

const clamp = (span: GridSpan, columns: number) => (span === "full" ? columns : Math.min(span, columns));

const columnCount = (spec: GridColumnSpec) => (typeof spec === "number" ? Math.max(1, spec) : spec.length);

const webTemplate = (spec: GridColumnSpec) =>
  typeof spec === "number" ? `repeat(${Math.max(1, spec)}, minmax(0, 1fr))` : spec.join(" ");

// A track as the flex item native lays it out as: a share is a flex weight, a fixed size is a
// width, and `auto` and `minmax` keep their content size or take an equal share.
const nativeTrack = (track: GridTrack): ComponentProps<typeof View> => {
  if (track === "auto") return { flexShrink: 0 };
  const share = /^([\d.]+)fr$/.exec(track);
  if (share) return { flex: Number(share[1]), minWidth: 0 };
  const fixed = /^([\d.]+)(px|rem)$/.exec(track);
  if (fixed) return { flexShrink: 0, width: Number(fixed[1]) * (fixed[2] === "rem" ? 16 : 1) };
  return { flex: 1, minWidth: 0 };
};

/**
 * Items in equal columns whose count follows the window: a form's two-column fields, a grid of
 * swatches or readouts. Where `Stack` lays out one line, a Grid lays out rows and columns. Gap and
 * column count come from the spacing tokens and the same breakpoints `Stack` answers to. An item
 * that covers more than one column is a `GridItem`; any other child covers one.
 *
 * On the web it is a CSS grid of `minmax(0, 1fr)` tracks, as Tailwind's `grid-cols-N` draws them,
 * or of the listed tracks, so a `ul` or `dl` keeps its own structure. Native has no grid, so it
 * lays the same cells out as rows of flex items, the last row padded so a short row keeps its
 * column widths.
 */
const Grid = ({
  align = "stretch",
  children,
  columnGap,
  columns = 1,
  gap = "none",
  render,
  rowGap,
  style,
  ...props
}: GridProps) => {
  const active = useActiveBreakpoint(
    [align, columns, gap, rowGap, columnGap].some((value) => isResponsive<unknown>(value)),
  );
  const spec = resolve(columns, active);
  const count = columnCount(spec);
  const resolvedGap = resolve(gap, active);
  const resolvedRowGap = space(rowGap === undefined ? resolvedGap : resolve(rowGap, active));
  const resolvedColumnGap = space(columnGap === undefined ? resolvedGap : resolve(columnGap, active));
  const alignment = alignItems[resolve(align, active)];

  if (isWeb) {
    const template: CSSProperties = { gridTemplateColumns: webTemplate(spec) };
    return (
      <GridColumns.Provider value={count}>
        <View
          {...props}
          alignItems={alignment}
          columnGap={resolvedColumnGap}
          render={render}
          rowGap={resolvedRowGap}
          // `display: grid` is not a Tamagui prop, and the grid's tracks are a CSS value, not a token.
          style={{ ...template, display: "grid", ...(style as CSSProperties) }}
        >
          {children}
        </View>
      </GridColumns.Provider>
    );
  }

  const rows = packRows(
    Children.toArray(children),
    count,
    active,
    typeof spec === "number" ? undefined : spec,
  );
  return (
    <GridColumns.Provider value={count}>
      <View {...props} gap={resolvedRowGap}>
        {rows.map((row) => (
          <View
            alignItems={alignment}
            flexDirection="row"
            flexShrink={0}
            gap={resolvedColumnGap}
            key={row.key}
          >
            {row.cells}
          </View>
        ))}
      </View>
    </GridColumns.Provider>
  );
};

type Row = { cells: ReactNode[]; key: string };

// Fills rows left to right with each child's span, starting a new row when one would overflow, and
// pads the last so its cells keep their column widths.
const packRows = (
  children: ReactNode[],
  columns: number,
  active: Parameters<typeof resolve>[1],
  tracks?: readonly GridTrack[],
): Row[] => {
  const rows: Row[] = [];
  let used = 0;
  let current: ReactNode[] = [];
  const flush = () => {
    if (current.length === 0) return;
    const padding =
      used < columns ? [<View flex={columns - used} key="padding" minWidth={0} />] : ([] as ReactElement[]);
    rows.push({ cells: [...current, ...padding], key: `row-${rows.length}` });
    current = [];
    used = 0;
  };
  for (const child of children) {
    const span = clamp(
      isValidElement<GridItemProps>(child) && child.type === GridItem
        ? resolve(child.props.span ?? 1, active)
        : 1,
      columns,
    );
    if (used + span > columns) flush();
    const track = tracks?.[used];
    current.push(
      isValidElement(child) && child.type === GridItem ? (
        child
      ) : (
        <View {...(track ? nativeTrack(track) : { flex: 1, minWidth: 0 })}>{child}</View>
      ),
    );
    used += span;
  }
  flush();
  return rows;
};

/** One item of a Grid, for the one that covers more than a column. */
const GridItem = ({ from, span = 1, style, ...props }: GridItemProps) => {
  const columns = useContext(GridColumns);
  const active = useActiveBreakpoint(isResponsive<GridSpan>(span) || isResponsive<number>(from));
  const resolved = resolve(span, active);
  const covered = clamp(resolved, columns);
  const start = from === undefined ? undefined : resolve(from, active);
  // From a named column, `full` runs to the last one (`2 / -1`) and a count spans from it.
  const gridColumn =
    start === undefined
      ? `span ${covered} / span ${covered}`
      : resolved === "full"
        ? `${start} / -1`
        : `${start} / span ${covered}`;
  return isWeb ? (
    <View {...props} style={{ gridColumn, ...(style as CSSProperties) }} />
  ) : (
    <View {...props} flex={covered} minWidth={0} />
  );
};

export type { GridColumnSpec, GridItemProps, GridProps, GridSpan, GridTrack };
export { Grid, GridItem };
