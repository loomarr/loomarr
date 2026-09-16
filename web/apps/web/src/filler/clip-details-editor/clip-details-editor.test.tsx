import type { ClipDTO } from "@loomarr/api/models/clipDTO";
import { getListTaxonomyMockHandler, getTagFillerClipMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { ClipDetailsEditor } from "./clip-details-editor";

const clip: ClipDTO = {
  hash: "correction-clip",
  name: "Ready clip",
  kind: "unclassified",
  durationMs: 30000,
  aiTagged: false,
  tagged: false,
  era: 1977,
  audience: "kids",
  brand: "Old advertiser",
  assertedTags: ["candy"],
  tags: ["candy", "food"],
  geographicScope: "national",
  country: "US",
  playCount: 0,
  playsCounted: true,
};
const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    {children}
  </QueryClientProvider>
);

describe("optional clip detail correction", () => {
  beforeEach(() =>
    server.use(
      getListTaxonomyMockHandler({
        taxa: [],
        totalClips: 1,
        taggedClips: 0,
        unclassifiedClips: 1,
        axisCoverage: [],
      }),
    ),
  );

  it("focuses the editor, tucks specialized fields away, and cancels without a write", async () => {
    let writes = 0;
    const onClose = vi.fn();
    server.use(
      getTagFillerClipMockHandler(() => {
        writes++;
        return clip;
      }),
    );
    render(<ClipDetailsEditor clip={clip} onClose={onClose} />, { wrapper });
    expect(screen.getByRole("region", { name: "Edit details: Ready clip" })).toHaveFocus();
    expect(screen.getByText("Location and broadcast details").closest("details")).not.toHaveAttribute("open");
    expect(screen.getByText("Topics and tags").closest("details")).not.toHaveAttribute("open");
    await userEvent.clear(screen.getByLabelText("Advertiser"));
    await userEvent.type(screen.getByLabelText("Advertiser"), "New advertiser");
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onClose).toHaveBeenCalledOnce();
    expect(writes).toBe(0);
  });

  it("saves direct tags and existing facts without asserting unknown kind or derived rollups", async () => {
    let payload: unknown;
    const onSaved = vi.fn();
    server.use(
      getTagFillerClipMockHandler(async ({ request }) => {
        payload = await request.json();
        return clip;
      }),
    );
    render(<ClipDetailsEditor clip={clip} onSaved={onSaved} />, { wrapper });
    await userEvent.clear(screen.getByLabelText("Advertiser"));
    await userEvent.type(screen.getByLabelText("Advertiser"), "New advertiser");
    expect(payload).toBeUndefined();
    await userEvent.click(screen.getByRole("button", { name: "Save details" }));
    await waitFor(() => expect(onSaved).toHaveBeenCalledOnce());
    expect(payload).toMatchObject({
      hash: "correction-clip",
      era: 1977,
      audience: "kids",
      brand: "New advertiser",
      tags: ["candy"],
      geography: { scope: "national", country: "US" },
    });
    expect(payload).not.toHaveProperty("kind");
  });
});
