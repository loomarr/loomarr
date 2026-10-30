import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { CatalogToolbar } from "./catalog-toolbar";

const props = {
  q: "",
  kind: "",
  audience: "",
  untagged: false,
  view: "grid",
  onFilterChange: vi.fn(),
  onViewChange: vi.fn(),
};

describe("CatalogToolbar", () => {
  it("writes the search text as a filter, and an emptied box as no filter", async () => {
    const onFilterChange = vi.fn();
    const { rerender } = render(<CatalogToolbar {...props} phone={false} onFilterChange={onFilterChange} />);

    await userEvent.type(screen.getByLabelText("Search"), "s");
    expect(onFilterChange).toHaveBeenLastCalledWith({ q: "s" });

    rerender(<CatalogToolbar {...props} phone={false} q="s" onFilterChange={onFilterChange} />);
    await userEvent.clear(screen.getByLabelText("Search"));
    expect(onFilterChange).toHaveBeenLastCalledWith({ q: undefined });
  });

  it("filters by kind and audience, and Any clears the filter", async () => {
    const onFilterChange = vi.fn();
    render(<CatalogToolbar {...props} phone={false} audience="kids" onFilterChange={onFilterChange} />);

    await userEvent.click(screen.getByRole("combobox", { name: "Kind" }));
    await userEvent.click(await screen.findByRole("option", { name: "PSA" }));
    expect(onFilterChange).toHaveBeenLastCalledWith({ kind: "psa" });

    await userEvent.click(screen.getByRole("combobox", { name: "Audience" }));
    await userEvent.click(await screen.findByRole("option", { name: "Any" }));
    expect(onFilterChange).toHaveBeenLastCalledWith({ audience: undefined });
  });

  it("toggles the untagged filter on and off", async () => {
    const onFilterChange = vi.fn();
    const { rerender } = render(<CatalogToolbar {...props} phone={false} onFilterChange={onFilterChange} />);

    await userEvent.click(screen.getByRole("button", { name: "Untagged only" }));
    expect(onFilterChange).toHaveBeenLastCalledWith({ untagged: true });

    rerender(<CatalogToolbar {...props} phone={false} untagged onFilterChange={onFilterChange} />);
    await userEvent.click(screen.getByRole("button", { name: "Untagged only" }));
    expect(onFilterChange).toHaveBeenLastCalledWith({ untagged: undefined });
  });

  it("reports a view change from the toggle", async () => {
    const onViewChange = vi.fn();
    render(<CatalogToolbar {...props} phone={false} onViewChange={onViewChange} />);

    await userEvent.click(screen.getByRole("radio", { name: "List" }));

    expect(onViewChange).toHaveBeenCalledWith("list");
  });

  it("folds the filters behind a Filters button on a phone", async () => {
    render(<CatalogToolbar {...props} phone />);

    expect(screen.queryByRole("combobox", { name: "Kind" })).not.toBeInTheDocument();
    const filters = screen.getByRole("button", { name: "Filters" });
    expect(filters).toHaveAttribute("aria-expanded", "false");

    await userEvent.click(filters);

    expect(filters).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("combobox", { name: "Kind" })).toBeVisible();
  });
});
