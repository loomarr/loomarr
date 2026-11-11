/// <reference lib="dom" />
import { createContext, type ReactNode, useContext, useState } from "react";

import { Text } from "../primitives";
import { type Density, semanticRadius, semanticSpace, typography } from "../tokens";
import type {
  TableCellProps,
  TableHeaderCellProps,
  TableProps,
  TableRowProps,
  TableSectionProps,
} from "./table.type";

// The web adapter: real <table> elements, so a screen reader gets rows, columns and header scopes
// and the browser lays the columns out. The look is the Help doc table's in the web mock (a bordered
// 8-radius box, a raised header row of small tracked mono labels, a quiet rule between rows), read
// through the theme's CSS variables so light and dark both hold.
const DensityContext = createContext<Density>("pointer");
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

const cellPadding = { paddingBlock: semanticSpace.control, paddingInline: semanticSpace.control };

// A bare string or number takes the cell's voice; anything else (a button, a status word) brings its own.
const isPlain = (children: ReactNode) => typeof children === "string" || typeof children === "number";

const Table = ({ caption, children, density = "pointer", framed = true, minWidth }: TableProps) => {
  const [ringed, setRinged] = useState(false);
  const text = typography[density].body;
  return (
    <DensityContext.Provider value={density}>
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
    </DensityContext.Provider>
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
  const density = useContext(DensityContext);
  return (
    <th
      colSpan={colSpan}
      scope="col"
      style={{
        ...cellPadding,
        backgroundColor: "var(--surfaceRaised)",
        borderBottom: framedRule,
        fontWeight: "normal",
        textAlign: align === "end" ? "right" : "left",
        width,
      }}
    >
      {isPlain(children) ? (
        <Text density={density} shout textRole="mono" tone="secondary" tracking={0.06}>
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
  verticalAlign = "middle",
  width,
}: TableCellProps) => {
  const density = useContext(DensityContext);
  return (
    <td
      colSpan={colSpan}
      style={{
        ...cellPadding,
        borderBottom: rowRule,
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
    </td>
  );
};

export { Table, TableBody, TableCell, TableHeader, TableHeaderCell, TableRow };
