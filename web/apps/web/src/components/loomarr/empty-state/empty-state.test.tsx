import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { EmptyState } from "./empty-state";

describe("EmptyState", () => {
  it("renders title, description and the single action", async () => {
    const onClick = vi.fn();
    render(
      <EmptyState
        title="Dead air"
        description="Create your first channel."
        action={{ label: "New channel", onClick }}
      />,
    );
    expect(screen.getByText("Dead air")).toBeInTheDocument();
    expect(screen.getByText("Create your first channel.")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "New channel" }));
    expect(onClick).toHaveBeenCalledOnce();
  });

  it("renders without an action", () => {
    render(<EmptyState title="Queue's clear" />);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});
