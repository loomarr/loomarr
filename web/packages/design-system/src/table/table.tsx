import { View } from "@tamagui/core";
import { Text } from "../text";
import { isPlainText as isPlain } from "./plain-text";
import type {
  TableCellProps,
  TableHeaderCellProps,
  TableProps,
  TableRowProps,
  TableSectionProps,
} from "./table.type";
import { TableContext, useTableContext } from "./table-context";

// The native adapter: a phone, tablet or TV has no table layout, so a row is a flex row and a cell
// takes a share of it by its `width` (the web adapter, table.web.tsx, is a real <table>). The look
// is the same: a bordered box, a raised header row of small tracked mono labels, a quiet row rule.
const Table = ({ caption, children, dense = false, density = "pointer", framed = true }: TableProps) => (
  <TableContext.Provider value={{ dense, density }}>
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
  </TableContext.Provider>
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

const cellBox = (width: number | undefined, align: "start" | "end", dense: boolean) =>
  ({
    alignItems: align === "end" ? "flex-end" : "flex-start",
    flexBasis: width ?? 0,
    flexGrow: width === undefined ? 1 : 0,
    flexShrink: 1,
    minWidth: 0,
    padding: dense ? "inline" : "control",
  }) as const;

const TableHeaderCell = ({ align = "start", children, width }: TableHeaderCellProps) => {
  const { dense, density } = useTableContext();
  return (
    <View
      {...cellBox(width, align, dense)}
      backgroundColor="surfaceRaised"
      borderBottomColor="borderDecorative"
      borderBottomWidth={1}
      role="columnheader"
    >
      {isPlain(children) ? (
        // Sentence case, as the caps rule keeps capitals for channel numbers, clocks and provenance.
        <Text density={density} textRole={dense ? "caption" : "mono"} tone="secondary">
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
  rowHeader = false,
  verticalAlign = "middle",
  width,
}: TableCellProps) => {
  const { dense, density } = useTableContext();
  return (
    <View
      {...cellBox(width, align, dense)}
      borderBottomColor="surfaceElevated"
      borderBottomWidth={1}
      justifyContent={verticalAlign === "top" ? "flex-start" : "center"}
      role={rowHeader ? "rowheader" : "cell"}
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
