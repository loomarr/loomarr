import type { EpisodeSelection } from "@loomarr/api/models/episodeSelection";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as rtlRender, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement, ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui";
import { ApprovalQueue } from "./approval-queue";

// The router is only reached on a successful approve that returns a channelId; these tests
// assert the REQUEST, so navigation is stubbed rather than mounting a router.
vi.mock("@tanstack/react-router", () => ({ useNavigate: () => vi.fn() }));

const makeWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <TooltipProvider>{children}</TooltipProvider>
    </QueryClientProvider>
  );
};

const render = (ui: ReactElement) => rtlRender(ui, { wrapper: makeWrapper() });

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });

const proposal = {
  id: "p1",
  createdBy: "kid",
  status: "submitted",
  proposal: {
    intent: { description: "90s action night" },
    rationale: "Heat leads it",
    lineup: [{ name: "Heat", year: 1995, mediaType: "movie", tmdbId: 949, inLibrary: true }],
    acquisitions: [
      { name: "The Simpsons", year: 1989, mediaType: "series", tvdbId: 71663, inLibrary: false },
    ],
  },
};

// Captures every approve body so a test can assert what the gate actually received.
const stubApi = () => {
  const approvals: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      if (typeof url === "string" && url.includes("/approve")) {
        approvals.push(init?.body ? JSON.parse(init.body as string) : undefined);
        return Promise.resolve(jsonResponse({ status: "approved", enqueued: 1 }));
      }
      if (typeof url === "string" && url.includes("/v1/search")) {
        return Promise.resolve(jsonResponse({ candidates: [] }));
      }
      if (typeof url === "string" && url.includes("/v1/proposals")) {
        return Promise.resolve(jsonResponse({ proposals: [proposal] }));
      }
      return Promise.resolve(jsonResponse({}));
    }),
  );
  return approvals;
};

const stubEpisodePreviewApi = (preview: EpisodeSelection) => {
  const approvals: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      if (typeof url === "string" && url.includes("/approve")) {
        approvals.push(init?.body ? JSON.parse(init.body as string) : undefined);
        return Promise.resolve(
          jsonResponse({ status: "approved", enqueued: 1, channelId: "channel-preview" }),
        );
      }
      if (typeof url === "string" && url.includes("/v1/search")) {
        return Promise.resolve(
          jsonResponse({
            candidates: [
              { name: "Added Series", year: 1989, mediaType: "series", tmdbId: 456, inLibrary: false },
            ],
          }),
        );
      }
      if (typeof url === "string" && url.includes("/v1/proposals")) {
        return Promise.resolve(
          jsonResponse({
            proposals: [
              {
                id: "p-preview",
                status: "submitted",
                episodeSelectionPreview: preview,
                proposal: {
                  intent: { description: "Movie-only proposal before a series is added" },
                  lineup: [{ name: "Heat", mediaType: "movie", tmdbId: 949, inLibrary: true }],
                  acquisitions: [],
                },
              },
            ],
          }),
        );
      }
      return Promise.resolve(jsonResponse({}));
    }),
  );
  return approvals;
};

afterEach(() => vi.unstubAllGlobals());

describe("ApprovalQueue — edit before approve (V25b)", () => {
  // The reachability assertion the phase gate asks for: the edit panel must actually MOUNT from
  // the queue. `ProposalReview.onEditItem` shipped with no production caller and the button
  // therefore never rendered — the recurring defect this repo's reachability tests exist for.
  it("mounts the edit surface from the queue", async () => {
    stubApi();
    render(<ApprovalQueue />);

    const toggle = await screen.findByRole("button", { name: /Review & edit picks/ });
    await userEvent.click(toggle);

    expect(screen.getByText("What gets approved")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Remove Heat" })).toBeInTheDocument();
    expect(screen.getByLabelText("Note to the requester")).toBeInTheDocument();
  });

  // ⚠ Approving an untouched proposal must send the SAME body it always did. The handler maps a
  // body with no drops/adds/note to a nil edit, so an empty object is the pre-V25 behaviour
  // exactly; anything else would record "approved with modifications: none" — false.
  it("approving unmodified sends an empty body, exactly as before", async () => {
    const approvals = stubApi();
    render(<ApprovalQueue />);

    await userEvent.click(await screen.findByRole("button", { name: /Approve/ }));

    await waitFor(() => expect(approvals).toHaveLength(1));
    expect(approvals[0]).toEqual({});
  });

  // The gate: an admin edits, then approves the EDITED version — one request, through the one
  // approval chokepoint. There is no separate "save edit" call, because the edit is a parameter
  // to `suggest.Approver` rather than a mutation of the proposal (§7 / D-K).
  it("approving after an edit sends the drop and the note on the same call", async () => {
    const approvals = stubApi();
    render(<ApprovalQueue />);

    await userEvent.click(await screen.findByRole("button", { name: /Review & edit picks/ }));
    await userEvent.click(screen.getByRole("button", { name: "Remove Heat" }));
    await userEvent.type(screen.getByLabelText("Note to the requester"), "too violent");
    await userEvent.click(screen.getByRole("button", { name: /Approve/ }));

    await waitFor(() => expect(approvals).toHaveLength(1));
    expect(approvals[0]).toEqual({ drop: ["movie:tmdb:949"], note: "too violent" });
  });

  it("undoing every edit returns to the unmodified body", async () => {
    const approvals = stubApi();
    render(<ApprovalQueue />);

    await userEvent.click(await screen.findByRole("button", { name: /Review & edit picks/ }));
    await userEvent.click(screen.getByRole("button", { name: "Remove Heat" }));
    await userEvent.click(screen.getByRole("button", { name: "Keep Heat" }));
    await userEvent.click(screen.getByRole("button", { name: /Approve/ }));

    await waitFor(() => expect(approvals).toHaveLength(1));
    expect(approvals[0]).toEqual({});
  });

  it.each<[EpisodeSelection, string]>([
    [{ mode: "highlights" }, "Curated highlights"],
    [{ mode: "holiday", holidays: ["christmas"] }, "christmas episodes"],
    [{ mode: "complete" }, "All episodes"],
  ])("labels a search-added series from server preview %o", async (preview, label) => {
    const approvals = stubEpisodePreviewApi(preview);
    render(<ApprovalQueue />);

    await userEvent.click(await screen.findByRole("button", { name: /Review & edit picks/ }));
    await userEvent.click(screen.getByRole("button", { name: /Add a title/ }));
    await userEvent.type(screen.getByRole("combobox"), "added series");
    await userEvent.click(await screen.findByText("Added Series"));

    expect(screen.getByText(label)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /^Approve$/ }));
    await waitFor(() => expect(approvals).toHaveLength(1));
    expect(approvals[0]).toEqual({
      add: [{ name: "Added Series", mediaType: "series", inLibrary: false, year: 1989, tmdbId: 456 }],
    });
  });
});
