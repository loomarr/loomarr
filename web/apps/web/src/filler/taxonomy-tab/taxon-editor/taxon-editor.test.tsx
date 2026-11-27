import type { TaxonDTO } from "@loomarr/api/models/taxonDTO";
import type { TaxonomyImpactDTO } from "@loomarr/api/models/taxonomyImpactDTO";
import { getPreviewTaxonomyEditMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";
import { TaxonEditor } from "./taxon-editor";
import type { Editor } from "./taxon-editor.type";

const taxon = (over: Partial<TaxonDTO> & Pick<TaxonDTO, "slug" | "label">): TaxonDTO => ({
  axis: "product",
  assertedClips: 0,
  matchedClips: 0,
  storedClips: 0,
  ...over,
});

const TAXA = [
  taxon({ slug: "food", label: "Food" }),
  taxon({ slug: "cereal", label: "Cereal", parent: "food", synonyms: ["breakfast-cereal"] }),
  taxon({ slug: "granola", label: "Granola", parent: "cereal" }),
  taxon({ slug: "drinks", label: "Drinks" }),
];

const NO_IMPACT: TaxonomyImpactDTO = {
  directStoredClips: 0,
  descendantStoredClips: 0,
  affectedStoredClips: 0,
  affectedPlayableClips: 0,
  descendants: [],
  savedChannelSelections: [],
  resolverTermsAdded: [],
  resolverTermsRemoved: [],
  deleteBlocked: false,
};

const renderEditor = (editor: Editor) => {
  const onClose = vi.fn();
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <TaxonEditor editor={editor} taxa={TAXA} onClose={onClose} />
    </QueryClientProvider>,
  );
  return { onClose };
};

describe("TaxonEditor", () => {
  it("adds a tag only after reviewing the impact of the exact command", async () => {
    const previews: unknown[] = [];
    let created: unknown;
    server.use(
      getPreviewTaxonomyEditMockHandler(async ({ request }) => {
        previews.push(await request.json());
        return NO_IMPACT;
      }),
      http.post("*/v1/taxonomy", async ({ request }) => {
        created = await request.json();
        return HttpResponse.json(taxon({ slug: "breakfast-cereal", label: "Breakfast" }));
      }),
    );
    const { onClose } = renderEditor({ mode: "create", axis: "product" });

    expect(screen.getByRole("region", { name: "Add tag" })).toBeInTheDocument();
    const review = screen.getByRole("button", { name: "Review changes" });
    expect(review).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Label"), { target: { value: "Breakfast" } });
    fireEvent.change(screen.getByLabelText("Slug"), { target: { value: "Breakfast Cereal" } });
    expect(screen.getByLabelText("Slug")).toHaveValue("breakfast-cereal");
    fireEvent.change(screen.getByLabelText("Classifier synonyms"), {
      target: { value: "brekkie, , morning food " },
    });
    await userEvent.click(review);

    expect(await screen.findByRole("heading", { name: "Review the impact" })).toBeInTheDocument();
    expect(previews).toEqual([
      {
        operation: "create",
        slug: "breakfast-cereal",
        label: "Breakfast",
        axis: "product",
        synonyms: ["brekkie", "morning food"],
        retiredAliases: [],
      },
    ]);
    await userEvent.click(screen.getByRole("button", { name: "Confirm add" }));
    await waitFor(() => expect(onClose).toHaveBeenCalledOnce());
    expect(created).toEqual({
      slug: "breakfast-cereal",
      label: "Breakfast",
      axis: "product",
      synonyms: ["brekkie", "morning food"],
      retiredAliases: [],
    });
  });

  it("drops a stale review as soon as a field changes", async () => {
    server.use(getPreviewTaxonomyEditMockHandler(NO_IMPACT));
    renderEditor({ mode: "edit", axis: "product", taxon: TAXA[3] });

    await userEvent.click(screen.getByRole("button", { name: "Review changes" }));
    expect(await screen.findByRole("button", { name: "Confirm update" })).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Label"), { target: { value: "Drinks!" } });
    expect(screen.queryByRole("heading", { name: "Review the impact" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Review changes" })).toBeEnabled();
  });

  it("keeps a term's slug and, while it has children, its axis fixed", () => {
    renderEditor({ mode: "edit", axis: "product", taxon: TAXA[1] });

    expect(screen.getByRole("region", { name: "Edit Cereal" })).toBeInTheDocument();
    expect(screen.getByLabelText("Slug")).toBeDisabled();
    expect(screen.getByLabelText("Classifier synonyms")).toHaveValue("breakfast-cereal");
    expect(screen.getByRole("button", { name: /^Axis/ })).toBeDisabled();
    expect(screen.getByText(/move this tag’s children before changing its axis/i)).toBeInTheDocument();
  });

  it("never offers a term or its descendants as its own parent", async () => {
    renderEditor({ mode: "edit", axis: "product", taxon: TAXA[1] });

    await userEvent.click(screen.getByRole("button", { name: /^Parent/ }));
    const parents = (await screen.findAllByRole("radio")).map((option) => option.textContent);
    expect(parents).toEqual(["Top level", "Drinks (drinks)", "Food (food)"]);
  });

  it("refuses to confirm a blocked removal", async () => {
    server.use(
      getPreviewTaxonomyEditMockHandler({ ...NO_IMPACT, deleteBlocked: true, directStoredClips: 2 }),
    );
    renderEditor({ mode: "edit", axis: "product", taxon: TAXA[3] });

    await userEvent.click(screen.getByRole("button", { name: "Review removal" }));
    expect(await screen.findByText(/retag 2 directly assigned stored clips/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Confirm removal" })).toBeDisabled();
    await userEvent.click(screen.getByRole("button", { name: "Back" }));
    expect(screen.queryByRole("heading", { name: "Review the impact" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Review removal" })).toBeEnabled();
  });

  it("closes on Cancel without asking for a review", async () => {
    const { onClose } = renderEditor({ mode: "create", axis: "seasonal" });
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onClose).toHaveBeenCalledOnce();
  });
});
