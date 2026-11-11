import { View } from "@tamagui/core";
import { createContext, type ReactNode, useContext } from "react";

import { Text } from "../primitives";
import type { Density } from "../tokens";
import type {
  TableCellProps,
  TableHeaderCellProps,
  TableProps,
  TableRowProps,
  TableSectionProps,
} from "./table.type";

// The native adapter: a phone, tablet or TV has no table layout, so a row is a flex row and a cell
// takes a share of it by its `width` (the web adapter, table.web.tsx, is a real <table>). The look
// is the same: a bordered box, a raised header row of small tracked mono labels, a quiet row rule.
const DensityContext = createContext<Density>("pointer");

// A bare string or number takes the cell's voice; anything else (a button, a status word) brings its own.
const isPlain = (children: ReactNode) => typeof children === "string" || typeof children === "number";

const Table = ({ caption, children, density = "pointer", framed = true }: TableProps) => (
  <DensityContext.Provider value={density}>
    <View
      aria-label={caption}
      borderColor="borderDecorative"
      borderRadius="control"
      borderWidth={framed ? 1 : 0}
      overflow="hidden"
      role="table"
      width="100%"
    >
      {/* The last row's rule is the frame's own bottom edge: pull it a px past the frame, which clips it. */}
      <View marginBottom={framed ? -1 : 0}>{children}</View>
    </View>
  </DensityContext.Provider>
);

const TableHeader = ({ children }: TableSectionProps) => <View role="rowgroup">{children}</View>;

const TableBody = ({ children }: TableSectionProps) => <View role="rowgroup">{children}</View>;

const TableRow = ({ children, dimmed = false, id, tone = "default" }: TableRowProps) => (
  <View
    alignItems="stretch"
    backgroundColor={tone === "raised" ? "surfaceRaised" : undefined}
    flexDirection="row"
    id={id}
    opacity={dimmed ? 0.6 : 1}
    role="row"
  >
    {children}
  </View>
);

const cellBox = (width: number | undefined, align: "start" | "end") =>
  ({
    alignItems: align === "end" ? "flex-end" : "flex-start",
    flexBasis: width ?? 0,
    flexGrow: width === undefined ? 1 : 0,
    flexShrink: 1,
    minWidth: 0,
    padding: "control",
  }) as const;

const TableHeaderCell = ({ align = "start", children, width }: TableHeaderCellProps) => {
  const density = useContext(DensityContext);
  return (
    <View
      {...cellBox(width, align)}
      backgroundColor="surfaceRaised"
      borderBottomColor="borderDecorative"
      borderBottomWidth={1}
      role="columnheader"
    >
      {isPlain(children) ? (
        <Text density={density} shout textRole="mono" tone="secondary" tracking={0.06}>
          {children}
        </Text>
      ) : (
        children
      )}
    </View>
  );
};

const TableCell = ({
  align = "start",
  children,
  mono = false,
  verticalAlign = "middle",
  width,
}: TableCellProps) => {
  const density = useContext(DensityContext);
  return (
    <View
      {...cellBox(width, align)}
      borderBottomColor="surfaceElevated"
      borderBottomWidth={1}
      justifyContent={verticalAlign === "top" ? "flex-start" : "center"}
      role="cell"
    >
      {isPlain(children) ? (
        <Text density={density} textRole={mono ? "mono" : "body"}>
          {children}
        </Text>
      ) : (
        children
      )}
    </View>
  );
};

export { Table, TableBody, TableCell, TableHeader, TableHeaderCell, TableRow };
