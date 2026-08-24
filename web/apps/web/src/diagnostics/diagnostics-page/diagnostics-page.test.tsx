import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { DiagnosticsPage, type DiagnosticsSearch } from "./diagnostics-page";

const search: DiagnosticsSearch = {
  view: "process",
  range: "1h",
  order: "newest",
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
  it("uses the site navigation treatment for all three operator views", async () => {
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
    expect(screen.queryByRole("button", { name: "Back to Logs" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Logs" })).toHaveClass("cursor-pointer");
    expect(screen.getByRole("button", { name: "Current Health" })).toHaveClass("cursor-pointer");
    const processes = screen.getByRole("button", { name: "Media processes" });
    expect(processes).toHaveClass("cursor-pointer", "bg-signal-tint-15", "text-signal");

    await userEvent.click(screen.getByRole("button", { name: "Logs" }));
    expect(onSearchChange).toHaveBeenCalledWith({ ...search, view: "logs", processId: undefined });
  });
});
