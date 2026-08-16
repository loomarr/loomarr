import { getListTaxonomyMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { TaxonomyTab } from "./taxonomy-tab";

const renderTab = () => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const rootRoute = createRootRoute({ component: () => <TaxonomyTab isAdmin /> });
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
    );
    renderTab();

    expect(await screen.findByText("10 / 12")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Products & topics" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Browse 5 without" })).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Cereal" }));
    const editor = await screen.findByRole("region", { name: "Edit Cereal" });
    expect(within(editor).getByLabelText("Slug")).toBeDisabled();
    expect(within(editor).getByText(/retag 2 stored clips/i)).toBeInTheDocument();
  });
});
