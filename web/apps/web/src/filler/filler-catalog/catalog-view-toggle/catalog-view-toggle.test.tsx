import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { CatalogViewToggle } from "./catalog-view-toggle";

describe("CatalogViewToggle", () => {
  it("is one radio group with the current view checked and the only tab stop", () => {
    render(<CatalogViewToggle view="grid" phone={false} onChange={vi.fn()} />);

    expect(screen.getByRole("radiogroup", { name: "Clip view" })).toBeVisible();
    expect(screen.getByRole("radio", { name: "Grid" })).toBeChecked();
    expect(screen.getByRole("radio", { name: "Grid" })).toHaveAttribute("tabindex", "0");
    expect(screen.getByRole("radio", { name: "List" })).not.toBeChecked();
    expect(screen.getByRole("radio", { name: "List" })).toHaveAttribute("tabindex", "-1");
  });

  it("chooses a view by click", async () => {
    const onChange = vi.fn();
    render(<CatalogViewToggle view="grid" phone={false} onChange={onChange} />);

    await userEvent.click(screen.getByRole("radio", { name: "List" }));

    expect(onChange).toHaveBeenCalledWith("list");
  });

  it("moves the choice and focus with the arrow keys, wrapping at the ends", async () => {
    const onChange = vi.fn();
    render(<CatalogViewToggle view="grid" phone={false} onChange={onChange} />);
    screen.getByRole("radio", { name: "Grid" }).focus();

    await userEvent.keyboard("{ArrowRight}");
    expect(onChange).toHaveBeenLastCalledWith("list");
    expect(screen.getByRole("radio", { name: "List" })).toHaveFocus();

    await userEvent.keyboard("{ArrowRight}");
    expect(onChange).toHaveBeenLastCalledWith("grid");
    expect(screen.getByRole("radio", { name: "Grid" })).toHaveFocus();

    await userEvent.keyboard("{ArrowLeft}");
    expect(onChange).toHaveBeenLastCalledWith("list");

    await userEvent.keyboard("{Home}");
    expect(onChange).toHaveBeenLastCalledWith("grid");
    await userEvent.keyboard("{End}");
    expect(onChange).toHaveBeenLastCalledWith("list");
  });

  it("ignores other keys", async () => {
    const onChange = vi.fn();
    render(<CatalogViewToggle view="grid" phone={false} onChange={onChange} />);
    screen.getByRole("radio", { name: "Grid" }).focus();

    await userEvent.keyboard("a");

    expect(onChange).not.toHaveBeenCalled();
  });

  it("shows icons only on a phone, keeping the accessible names", () => {
    render(<CatalogViewToggle view="list" phone onChange={vi.fn()} />);

    expect(screen.getByRole("radio", { name: "List" })).toBeChecked();
    expect(screen.queryByText("List")).not.toBeInTheDocument();
  });
});
