import type { TaxonDTO } from "@loomarr/api/models/taxonDTO";
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from "@tanstack/react-router";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { render } from "@/test/render";
import { TaxonTree, treeForAxis } from "./taxon-tree";

const taxon = (over: Partial<TaxonDTO> & Pick<TaxonDTO, "slug" | "label">): TaxonDTO => ({
  axis: "product",
  assertedClips: 0,
  matchedClips: 0,
  storedClips: 0,
  ...over,
});

const TAXA = [
  taxon({ slug: "food", label: "Food", matchedClips: 3 }),
  taxon({ slug: "snacks", label: "Snacks", parent: "food", matchedClips: 1 }),
  taxon({
    slug: "cereal",
    label: "Cereal",
    parent: "food",
    synonyms: ["breakfast-cereal"],
    retiredAliases: ["grain"],
    assertedClips: 1,
    matchedClips: 2,
  }),
  taxon({ slug: "animated", label: "Animated", axis: "presentation", matchedClips: 1 }),
  // A parent on another axis is not a parent here, so the node stays at the top level.
  taxon({ slug: "orphan", label: "Orphan", parent: "animated" }),
];

const inRouter = (ui: ReactNode) => {
  const rootRoute = createRootRoute({ component: () => ui });
  render(<RouterProvider router={createRouter({ routeTree: rootRoute, history: createMemoryHistory() })} />);
};

describe("treeForAxis", () => {
  it("nests one axis by parent and sorts each level by label", () => {
    const tree = treeForAxis(TAXA, "product");
    expect(tree.map((node) => node.taxon.slug)).toEqual(["food", "orphan"]);
    expect(tree[0]?.children.map((node) => node.taxon.slug)).toEqual(["cereal", "snacks"]);
    expect(treeForAxis(TAXA, "presentation").map((node) => node.taxon.slug)).toEqual(["animated"]);
    expect(treeForAxis(TAXA, "seasonal")).toEqual([]);
  });
});

describe("TaxonTree", () => {
  it("renders a labelled nested list with clip counts and classifier terms", async () => {
    inRouter(<TaxonTree nodes={treeForAxis(TAXA, "product")} axis="product" isAdmin onEdit={vi.fn()} />);

    const tree = await screen.findByRole("list", { name: "Products & topics vocabulary" });
    expect(within(tree).getAllByRole("list")).toHaveLength(1);
    expect(within(tree).getByRole("link", { name: "3 clips" })).toBeInTheDocument();
    expect(within(tree).getByRole("link", { name: "1 clip" })).toBeInTheDocument();
    expect(within(tree).getByText("1 direct")).toBeInTheDocument();
    await userEvent.click(within(tree).getByText("2 classifier terms"));
    expect(within(tree).getByText("Synonyms: breakfast-cereal")).toBeInTheDocument();
    expect(within(tree).getByText("Retired slugs: grain")).toBeInTheDocument();
  });

  it("lets an administrator open a term for editing", async () => {
    const onEdit = vi.fn();
    inRouter(<TaxonTree nodes={treeForAxis(TAXA, "product")} axis="product" isAdmin onEdit={onEdit} />);
    await userEvent.click(await screen.findByRole("button", { name: "Cereal" }));
    expect(onEdit).toHaveBeenCalledWith(expect.objectContaining({ slug: "cereal" }));
  });

  it("keeps terms read-only for a member", async () => {
    inRouter(
      <TaxonTree nodes={treeForAxis(TAXA, "product")} axis="product" isAdmin={false} onEdit={vi.fn()} />,
    );
    expect(await screen.findByRole("button", { name: "Food" })).toBeDisabled();
  });
});
