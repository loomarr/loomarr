/// <reference lib="dom" />
import { useState } from "react";

import { Text } from "../primitives";
import { semanticRadius, semanticSpace, typography } from "../tokens";
import { isPlainText as isPlain } from "./plain-text";
import type {
  TableCellProps,
  TableHeaderCellProps,
  TableProps,
  TableRowProps,
  TableSectionProps,
} from "./table.type";
import { TableContext, useTableContext } from "./table-context";

// The web adapter: real <table> elements, so a screen reader gets rows, columns and header scopes
// and the browser lays the columns out. The look is the Help doc table's in the web mock (a bordered
// 8-radius box, a raised header row of small tracked mono labels, a quiet rule between rows), read
// through the theme's CSS variables so light and dark both hold.
const framedRule = "1px solid var(--borderDecorative)";
const rowRule = "1px solid var(--surfaceElevated)";

const visuallyHidden = {
  border: 0,
  clip: "rect(0 0 0 0)",
  height: 1,
  margin: -1,
  overflow: "hidden",
  padding: 0,
  position: "absolute",
  whiteSpace: "nowrap",
  width: 1,
} as const;

const cellPadding = (dense: boolean) => {
  const space = dense ? semanticSpace.inline : semanticSpace.control;
  return { paddingBlock: space, paddingInline: space };
};

const Table = ({
  caption,
  children,
  dense = false,
  density = "pointer",
  framed = true,
  minWidth,
}: TableProps) => {
  const [ringed, setRinged] = useState(false);
  const text = typography[density].body;
  return (
    <TableContext.Provider value={{ dense, density }}>
      <div
        style={{
          border: framed ? framedRule : undefined,
          borderRadius: framed ? semanticRadius.control : undefined,
          // The last row's rule is the frame's own bottom edge: the scroller reaches one px past
          // it, the frame clips that px.
          overflow: "hidden",
        }}
      >
        <section
          aria-label={caption}
          onBlur={() => setRinged(false)}
          onFocus={(event) => setRinged(event.currentTarget.matches(":focus-visible"))}
          style={{
            marginBottom: framed ? -1 : undefined,
            outline: ringed ? `${density === "tv" ? 3 : 2}px solid var(--actionFocus)` : "none",
            outlineOffset: -2,
            overflowX: "auto",
          }}
          // biome-ignore lint/a11y/noNoninteractiveTabindex: a sideways scroller must take focus so the keyboard can scroll it.
          tabIndex={0}
        >
          <table
            style={{
              borderCollapse: "collapse",
              color: "var(--contentPrimary)",
              fontFamily: typography.family.body.web,
              fontSize: text.size,
              lineHeight: `${text.lineHeight}px`,
              minWidth,
              width: "100%",
            }}
          >
            <caption style={visuallyHidden}>{caption}</caption>
            {children}
          </table>
        </section>
      </div>
    </TableContext.Provider>
  );
};

const TableHeader = ({ children }: TableSectionProps) => <thead>{children}</thead>;

const TableBody = ({ children }: TableSectionProps) => <tbody>{children}</tbody>;

const TableRow = ({ children, dimmed = false, id, tone = "default" }: TableRowProps) => (
  <tr
    id={id}
    style={{
      backgroundColor: tone === "raised" ? "var(--surfaceRaised)" : undefined,
      opacity: dimmed ? 0.6 : undefined,
    }}
  >
    {children}
  </tr>
);

const TableHeaderCell = ({ align = "start", children, colSpan, width }: TableHeaderCellProps) => {
  const { dense, density } = useTableContext();
  return (
    <th
      colSpan={colSpan}
      scope="col"
      style={{
        ...cellPadding(dense),
        backgroundColor: "var(--surfaceRaised)",
        borderBottom: framedRule,
        fontWeight: "normal",
        textAlign: align === "end" ? "right" : "left",
        width,
      }}
    >
      {isPlain(children) ? (
        // Dense: the plain caption, which fits a narrow column better than tracked capitals.
        <Text
          density={density}
          shout={!dense}
          textRole={dense ? "caption" : "mono"}
          tone="secondary"
          tracking={dense ? undefined : 0.06}
        >
          {children}
        </Text>
      ) : (
        children
      )}
    </th>
  );
};

const TableCell = ({
  align = "start",
  children,
  colSpan,
  mono = false,
  rowHeader = false,
  verticalAlign = "middle",
  width,
}: TableCellProps) => {
  const { dense, density } = useTableContext();
  const Element = rowHeader ? "th" : "td";
  return (
    <Element
      colSpan={colSpan}
      scope={rowHeader ? "row" : undefined}
      style={{
        ...cellPadding(dense),
        borderBottom: rowRule,
        fontWeight: "normal",
        textAlign: align === "end" ? "right" : "left",
        verticalAlign,
        width,
      }}
    >
      {mono && isPlain(children) ? (
        <Text density={density} textRole="mono">
          {children}
        </Text>
      ) : (
        children
      )}
    </Element>
  );
};

export { Table, TableBody, TableCell, TableHeader, TableHeaderCell, TableRow };
