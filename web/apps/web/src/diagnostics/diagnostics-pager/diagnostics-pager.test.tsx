import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { DiagnosticsPager } from "./diagnostics-pager";

describe("DiagnosticsPager", () => {
  it("labels time direction and only invokes available navigation", async () => {
    const onNewer = vi.fn();
    const onOlder = vi.fn();
    render(
      <DiagnosticsPager
        label="Log pages"
        page={2}
        itemLabel="logs"
        canGoNewer
        canGoOlder={false}
        onNewer={onNewer}
        onOlder={onOlder}
      />,
    );

    expect(screen.getByText("Page 2")).toBeInTheDocument();
    expect(screen.getByText("Up to 50 logs")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Newer" }));
    expect(onNewer).toHaveBeenCalledOnce();
    expect(screen.getByRole("button", { name: "Older" })).toBeDisabled();
    expect(onOlder).not.toHaveBeenCalled();
  });
});
