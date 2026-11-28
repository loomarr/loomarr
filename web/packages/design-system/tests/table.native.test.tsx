// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
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

// The native Table (table.tsx): views carrying the table roles, there being no <table> to use.

afterEach(cleanup);

describe("Table (native)", () => {
  it("announces its rows, column headers, row header and cells by role", () => {
    render(
      <LoomarrProvider>
        <Table caption="Tasks">
          <TableHeader>
            <TableRow>
              <TableHeaderCell>Task</TableHeaderCell>
              <TableHeaderCell>Last run</TableHeaderCell>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow>
              <TableCell rowHeader>Library scan</TableCell>
              <TableCell>Today</TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </LoomarrProvider>,
    );

    expect(screen.getByRole("table", { name: "Tasks" })).toBeTruthy();
    expect(screen.getAllByRole("row")).toHaveLength(2);
    expect(screen.getAllByRole("columnheader").map((cell) => cell.textContent)).toEqual(["Task", "Last run"]);
    expect(screen.getByRole("rowheader").textContent).toBe("Library scan");
    expect(screen.getByRole("cell").textContent).toBe("Today");
  });
});
