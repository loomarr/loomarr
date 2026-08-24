import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { DiagnosticsPage, type DiagnosticsSearch } from "./diagnostics-page";

const search: DiagnosticsSearch = {
  view: "playout",
  range: "1h",
  level: "all",
  source: "all",
  subsystem: "",
  text: "",
  processRange: "1h",
  processStatus: "all",
  processPurpose: "",
  processChannelId: "",
  processJobId: "",
};

describe("DiagnosticsPage", () => {
  it("composes the three operator views and reflects tab choices", async () => {
    const onSearchChange = vi.fn();
    render(
      <DiagnosticsPage
        search={search}
        onSearchChange={onSearchChange}
        playout={<p>Live Process evidence</p>}
      />,
    );

    expect(screen.getByRole("heading", { name: "Diagnostics" })).toBeInTheDocument();
    expect(screen.getByText("Live Process evidence")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Playout" })).toHaveAttribute("aria-current", "page");

    await userEvent.click(screen.getByRole("button", { name: "Application" }));
    expect(onSearchChange).toHaveBeenCalledWith({ ...search, view: "application" });
  });
});
