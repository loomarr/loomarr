// @vitest-environment jsdom
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";

import {
  LoomarrProvider,
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
} from "../index";

// The web Table is a real <table>. What it owes a screen reader and a keyboard user is pinned here:
// the table is named, columns have header scopes, rows read their cells, and a table that scrolls
// sideways can be focused and scrolled from the keyboard.

afterEach(cleanup);

const renderTable = (props: Partial<React.ComponentProps<typeof Table>> = {}) =>
  render(
    <LoomarrProvider>
      <Table caption="Current health checks" {...props}>
        <TableHeader>
          <TableRow>
            <TableHeaderCell>Check</TableHeaderCell>
            <TableHeaderCell align="end" width={120}>
              Status
            </TableHeaderCell>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow>
            <TableCell mono>database</TableCell>
            <TableCell align="end">Healthy</TableCell>
          </TableRow>
          <TableRow dimmed id="detail" tone="raised">
            <TableCell colSpan={2} verticalAlign="top">
              Opened detail
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </LoomarrProvider>,
  );

describe("Table (web)", () => {
  it("is a named table whose columns have header scopes and whose rows read their cells", () => {
    renderTable();
    const table = screen.getByRole("table", { name: "Current health checks" });

    const headers = within(table).getAllByRole("columnheader");
    expect(headers.map((header) => header.textContent)).toEqual(["Check", "Status"]);
    expect(headers.every((header) => header.getAttribute("scope") === "col")).toBe(true);

    const row = within(table).getByRole("row", { name: /database/ });
    expect(
      within(row)
        .getAllByRole("cell")
        .map((cell) => cell.textContent),
    ).toEqual(["database", "Healthy"]);
  });

  it("gives a header the header voice when its text arrives as several strings", () => {
    render(
      <LoomarrProvider>
        <Table caption="Markdown table">
          <TableHeader>
            <TableRow>
              <TableHeaderCell>{["Symptom", " ", "or cause"]}</TableHeaderCell>
            </TableRow>
          </TableHeader>
        </Table>
      </LoomarrProvider>,
    );

    expect(screen.getByRole("columnheader").querySelector("span")?.textContent).toBe("Symptom or cause");
  });

  it("carries a row's id, dimming and a cell's span for a detail row", () => {
    const { container } = renderTable();
    const detail = container.querySelector<HTMLTableRowElement>("#detail");

    expect(detail?.style.opacity).toBe("0.6");
    expect(detail?.querySelector("td")?.getAttribute("colspan")).toBe("2");
    expect(screen.getByRole("columnheader", { name: "Status" }).style.width).toBe("120px");
  });

  it("frames the table by default and lets a table inside a card go without", () => {
    // The frame is the scroller's parent.
    const frameBorder = () => screen.getByRole("region").parentElement?.style.border;

    renderTable();
    expect(frameBorder()).not.toBe("");
    cleanup();

    renderTable({ framed: false });
    expect(frameBorder()).toBe("");
  });

  it("lets the keyboard reach a sideways scroller and ringed it", async () => {
    renderTable({ minWidth: 704 });
    const scroller = screen.getByRole("region", { name: "Current health checks" });

    expect(scroller.tabIndex).toBe(0);
    expect(scroller.querySelector("table")?.style.minWidth).toBe("704px");
    await userEvent.tab();
    expect(document.activeElement).toBe(scroller);
  });
});
