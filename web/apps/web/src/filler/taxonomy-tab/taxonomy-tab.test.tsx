import { getListTaxonomyMockHandler, getPreviewTaxonomyEditMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { TaxonomyTab } from "./taxonomy-tab";

const renderTab = (isAdmin = true) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const rootRoute = createRootRoute({ component: () => <TaxonomyTab isAdmin={isAdmin} /> });
  const router = createRouter({ routeTree: rootRoute, history: createMemoryHistory() });
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
};

describe("TaxonomyTab", () => {
  it("renders independent-axis coverage and an editable hierarchy", async () => {
    server.use(
      getListTaxonomyMockHandler({
        totalClips: 12,
        taggedClips: 10,
        unclassifiedClips: 2,
        axisCoverage: [
          { axis: "product", taggedClips: 7, untaggedClips: 5 },
          { axis: "format", taggedClips: 3, untaggedClips: 9 },
          { axis: "seasonal", taggedClips: 1, untaggedClips: 11 },
          { axis: "audience-cue", taggedClips: 2, untaggedClips: 10 },
        ],
        taxa: [
          { slug: "food", label: "Food", axis: "product", assertedClips: 0, matchedClips: 3, storedClips: 0 },
          {
            slug: "cereal",
            label: "Cereal",
            axis: "product",
            parent: "food",
            synonyms: ["breakfast-cereal"],
            assertedClips: 2,
            matchedClips: 2,
            storedClips: 2,
          },
        ],
      }),
      getPreviewTaxonomyEditMockHandler({
        directStoredClips: 2,
        descendantStoredClips: 4,
        affectedStoredClips: 6,
        affectedPlayableClips: 5,
        descendants: [{ slug: "cereal", label: "Cereal" }],
        savedChannelSelections: [{ id: "channel-1", number: 7, name: "Saturday Morning" }],
        resolverTermsAdded: [],
        resolverTermsRemoved: ["food"],
        deleteBlocked: true,
      }),
    );
    renderTab();

    expect(await screen.findByText("10 / 12")).toBeInTheDocument();
    expect(screen.getAllByRole("heading", { name: "Products & topics" })).toHaveLength(2);
    expect(screen.getByRole("link", { name: /browse 5 without/i })).toBeInTheDocument();

    await userEvent.click(screen.getByText("Manage vocabulary"));
    await userEvent.click(screen.getByRole("button", { name: "Cereal" }));
    const editor = await screen.findByRole("region", { name: "Edit Cereal" });
    expect(within(editor).getByLabelText("Slug")).toBeDisabled();
    await userEvent.click(within(editor).getByRole("button", { name: "Review removal" }));
    expect(
      await within(editor).findByText(/6 stored clips may have different inherited classification/i),
    ).toBeInTheDocument();
    expect(within(editor).getByText("Channel 7: Saturday Morning")).toBeInTheDocument();
    expect(within(editor).getByText(/stops resolving: food/i)).toBeInTheDocument();
    expect(within(editor).getByText(/retag 2 directly assigned stored clips/i)).toBeInTheDocument();
    expect(within(editor).getByRole("button", { name: "Confirm removal" })).toBeDisabled();
  });

  it("keeps an empty vocabulary useful and leaves members read-only", async () => {
    server.use(
      getListTaxonomyMockHandler({
        totalClips: 3,
        taggedClips: 0,
        unclassifiedClips: 3,
        axisCoverage: [],
        taxa: [],
      }),
    );
    renderTab(false);

    expect(await screen.findByRole("heading", { name: "Classification coverage" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /review 3 unclassified clips/i })).toBeInTheDocument();
    await userEvent.click(screen.getByText("Browse vocabulary"));
    expect(screen.getAllByText("No terms yet.")).toHaveLength(4);
    expect(screen.queryByRole("button", { name: /add/i })).not.toBeInTheDocument();
  });

  it("preserves semantic list hierarchy for deep trees and long labels", async () => {
    server.use(
      getListTaxonomyMockHandler({
        totalClips: 1,
        taggedClips: 1,
        unclassifiedClips: 0,
        axisCoverage: [],
        taxa: [
          { slug: "root", label: "Root", axis: "product", assertedClips: 0, matchedClips: 1, storedClips: 0 },
          {
            slug: "child",
            label: "A deliberately long child classification label that must remain readable",
            axis: "product",
            parent: "root",
            assertedClips: 0,
            matchedClips: 1,
            storedClips: 0,
          },
          {
            slug: "grandchild",
            label: "Grandchild",
            axis: "product",
            parent: "child",
            assertedClips: 1,
            matchedClips: 1,
            storedClips: 1,
          },
        ],
      }),
    );
    renderTab(false);
    await screen.findByRole("heading", { name: "Classification coverage" });
    await userEvent.click(screen.getByText("Browse vocabulary"));

    const tree = screen.getByRole("list", { name: "Products & topics vocabulary" });
    expect(within(tree).getAllByRole("list")).toHaveLength(2);
    expect(
      within(tree).getByRole("button", {
        name: "A deliberately long child classification label that must remain readable",
      }),
    ).toBeDisabled();
  });
});
