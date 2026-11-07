import type { ClipDTO } from "@loomarr/api/models/clipDTO";
import { getListTaxonomyMockHandler, getTagFillerClipMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
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
  needsDetails: false,
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
    // An unchanged name is not sent: saving another detail must not record it as curated (#1990).
    expect(payload).not.toHaveProperty("name");
  });

  it("does not send an untouched name that the catalog stores with surrounding spaces", async () => {
    let payload: unknown;
    const onSaved = vi.fn();
    server.use(
      getTagFillerClipMockHandler(async ({ request }) => {
        payload = await request.json();
        return clip;
      }),
    );
    render(<ClipDetailsEditor clip={{ ...clip, name: " Ready clip " }} onSaved={onSaved} />, { wrapper });
    await userEvent.click(screen.getByRole("button", { name: "Save details" }));
    await waitFor(() => expect(onSaved).toHaveBeenCalledOnce());
    expect(payload).not.toHaveProperty("name");
  });

  it("saves a renamed clip's trimmed name", async () => {
    let payload: unknown;
    const onSaved = vi.fn();
    server.use(
      getTagFillerClipMockHandler(async ({ request }) => {
        payload = await request.json();
        return { ...clip, name: "Fizzbolt Summer Sale" };
      }),
    );
    render(<ClipDetailsEditor clip={clip} onSaved={onSaved} />, { wrapper });
    const name = screen.getByLabelText("Name");
    expect(name).toHaveValue("Ready clip");
    await userEvent.clear(name);
    await userEvent.type(name, "  Fizzbolt Summer Sale ");
    await userEvent.click(screen.getByRole("button", { name: "Save details" }));
    await waitFor(() => expect(onSaved).toHaveBeenCalledOnce());
    expect(payload).toMatchObject({ hash: "correction-clip", name: "Fizzbolt Summer Sale" });
  });

  it("pre-fills a local clip's call sign and sends it", async () => {
    let payload: unknown;
    const onSaved = vi.fn();
    server.use(
      getTagFillerClipMockHandler(async ({ request }) => {
        payload = await request.json();
        return clip;
      }),
    );
    const local: ClipDTO = {
      ...clip,
      geographicScope: "local",
      station: "fcc:12345",
      stationCallSign: "WQXT",
      stationLabel: "WQXT · Harborview, OH",
    };
    render(<ClipDetailsEditor clip={local} onSaved={onSaved} />, { wrapper });
    const station = screen.getByLabelText("Station call sign");
    expect(station).toHaveValue("WQXT");
    await userEvent.clear(station);
    await userEvent.type(station, "WZZB-TV");
    await userEvent.click(screen.getByRole("button", { name: "Save details" }));
    await waitFor(() => expect(onSaved).toHaveBeenCalledOnce());
    expect(payload).toMatchObject({ geography: { scope: "local", country: "US", station: "WZZB-TV" } });
  });

  it("keeps a national clip's station by id when another detail changes", async () => {
    let payload: unknown;
    const onSaved = vi.fn();
    server.use(
      getTagFillerClipMockHandler(async ({ request }) => {
        payload = await request.json();
        return clip;
      }),
    );
    const credited: ClipDTO = {
      ...clip,
      geographicScope: "national",
      station: "fcc:12345",
      stationCallSign: "WQXT",
      stationLabel: "WQXT · Harborview, OH",
    };
    render(<ClipDetailsEditor clip={credited} onSaved={onSaved} />, { wrapper });
    expect(screen.getByLabelText("Station call sign (optional)")).toHaveValue("WQXT");
    await userEvent.clear(screen.getByLabelText("Advertiser"));
    await userEvent.type(screen.getByLabelText("Advertiser"), "Fizzbolt");
    await userEvent.click(screen.getByRole("button", { name: "Save details" }));
    await waitFor(() => expect(onSaved).toHaveBeenCalledOnce());
    expect(payload).toMatchObject({ geography: { scope: "national", station: "fcc:12345" } });
  });

  it("shows the server's reason when the call sign is not recognised", async () => {
    server.use(
      http.patch("*/v1/filler/tags", () =>
        HttpResponse.json(
          {
            title: "Unprocessable Entity",
            status: 422,
            detail: "No station is called WZZB-TV on that date.",
          },
          { status: 422, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    render(<ClipDetailsEditor clip={{ ...clip, geographicScope: "local" }} />, { wrapper });
    await userEvent.type(screen.getByLabelText("Station call sign"), "WZZB-TV");
    await userEvent.click(screen.getByRole("button", { name: "Save details" }));
    expect(await screen.findByText(/No station is called WZZB-TV/)).toBeInTheDocument();
  });
});
