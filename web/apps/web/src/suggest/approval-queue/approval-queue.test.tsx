import type { EpisodeSelection } from "@loomarr/api/models/episodeSelection";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as rtlRender, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement, ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui";
import { outlook } from "@/test/fixtures/outlook";
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
const stubApi = (
  assessment = outlook({
    state: "uncertain",
    programs: 0,
    unknownTitles: 2,
    uniqueRuntimeMs: 0,
    firstRepeatMs: null,
  }),
) => {
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
      if (typeof url === "string" && url.includes("/v1/discovery/feedback")) {
        return Promise.resolve(jsonResponse([]));
      }
      if (typeof url === "string" && url.endsWith("/outlook")) {
        return Promise.resolve(jsonResponse(assessment));
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
      if (typeof url === "string" && url.includes("/v1/discovery/feedback")) {
        return Promise.resolve(jsonResponse([]));
      }
      if (typeof url === "string" && url.endsWith("/outlook")) {
        return Promise.resolve(
          jsonResponse(
            outlook({
              state: "uncertain",
              programs: 0,
              unknownTitles: 2,
              uniqueRuntimeMs: 0,
              firstRepeatMs: null,
            }),
          ),
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
  it("keeps one explicit path from a thin outlook into the pick editor", async () => {
    const approvals = stubApi(outlook({ thin: true }));
    render(<ApprovalQueue />);
    expect(await screen.findByText(/Short lineup:/)).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: /Review & edit picks/ }));
    expect(screen.getByText("Titles")).toBeVisible();
    expect(screen.getByRole("checkbox", { name: "Include Heat" })).toBeVisible();
    expect(approvals).toEqual([]);
  });

  it("mounts the edit surface from the queue", async () => {
    stubApi();
    render(<ApprovalQueue />);

    const toggle = await screen.findByRole("button", { name: /Review & edit picks/ });
    await userEvent.click(toggle);

    expect(screen.getByText("Titles")).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Include Heat" })).toBeInTheDocument();
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
    await userEvent.click(screen.getByRole("checkbox", { name: "Include Heat" }));
    await userEvent.type(screen.getByLabelText("Note to the requester"), "too violent");
    await userEvent.click(screen.getByRole("button", { name: /Approve/ }));

    await waitFor(() => expect(approvals).toHaveLength(1));
    expect(approvals[0]).toEqual({ drop: ["movie:tmdb:949"], note: "too violent" });
  });

  it("undoing every edit returns to the unmodified body", async () => {
    const approvals = stubApi();
    render(<ApprovalQueue />);

    await userEvent.click(await screen.findByRole("button", { name: /Review & edit picks/ }));
    await userEvent.click(screen.getByRole("checkbox", { name: "Include Heat" }));
    await userEvent.click(screen.getByRole("checkbox", { name: "Include Heat" }));
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
    await userEvent.click(screen.getByRole("button", { name: "Add title" }));
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

// #1404 — pulls, names and the per-row pending state, each against the production shape the
// issue describes.
describe("ApprovalQueue — pulls, names, per-row pending", () => {
  const pull = {
    id: "pull_1",
    title: "Top up the 1990s",
    reason: "Saturday Mornings falls back to bumpers.",
    proposedBy: "ada",
    status: "pending",
    estimateClips: 52,
    candidateCount: 1,
    rejected: [],
    sources: [],
    createdAt: "2026-08-01T12:00:00Z",
    plan: [],
  };
  const second = {
    ...proposal,
    id: "p2",
    proposal: { ...proposal.proposal, intent: { description: "Noir" } },
  };

  // `holdApprove` leaves the approve request unanswered so the mutation stays pending.
  const stub = (opts: { proposals?: unknown[]; pulls?: () => Response; holdApprove?: boolean }) => {
    vi.stubGlobal(
      "fetch",
      vi.fn((url: string) => {
        if (url.includes("/approve")) {
          return opts.holdApprove ? new Promise<Response>(() => {}) : Promise.resolve(jsonResponse({}));
        }
        if (url.includes("/v1/filler/pulls")) {
          return Promise.resolve(opts.pulls ? opts.pulls() : jsonResponse({ pulls: [] }));
        }
        if (url.includes("/v1/discovery/feedback")) return Promise.resolve(jsonResponse([]));
        if (url.endsWith("/outlook")) return Promise.resolve(jsonResponse(outlook({ state: "uncertain" })));
        if (url.includes("/v1/proposals")) {
          return Promise.resolve(jsonResponse({ proposals: opts.proposals ?? [] }));
        }
        return Promise.resolve(jsonResponse({}));
      }),
    );
  };

  it("shows a pending pull even when there are no proposals", async () => {
    stub({ pulls: () => jsonResponse({ pulls: [pull] }) });
    render(<ApprovalQueue />);

    expect(await screen.findByText("Top up the 1990s")).toBeInTheDocument();
    expect(screen.queryByText("Queue's clear")).not.toBeInTheDocument();
  });

  it("is clear only when there are neither proposals nor pulls", async () => {
    stub({});
    render(<ApprovalQueue />);

    expect(await screen.findByText("Queue's clear")).toBeInTheDocument();
  });

  it("surfaces a filler-pull fetch error instead of hiding it (and is not 'clear')", async () => {
    stub({
      pulls: () =>
        new Response(JSON.stringify({ title: "Boom", detail: "pulls unavailable" }), {
          status: 500,
          headers: { "content-type": "application/json" },
        }),
    });
    render(<ApprovalQueue />);

    expect(await screen.findByText("pulls unavailable")).toBeInTheDocument();
    expect(screen.queryByText("Queue's clear")).not.toBeInTheDocument();
  });

  it("names the requester, never the raw user id", async () => {
    stub({
      proposals: [{ ...proposal, createdBy: "45dd1eae0a9b4f3c8d2e7f6a5b4c3d2e", createdByName: "Kid" }],
    });
    render(<ApprovalQueue />);

    expect(await screen.findByText("Requested by Kid")).toBeInTheDocument();
    expect(screen.queryByText(/45dd1eae/)).not.toBeInTheDocument();
  });

  it("marks only the row being approved as approving", async () => {
    stub({ proposals: [proposal, second], holdApprove: true });
    render(<ApprovalQueue />);

    // Scoped to the rows: with two approvable rows the bulk bar adds its own "Approve" button.
    const [list] = await screen.findAllByRole("list");
    if (!list) throw new Error("no proposal list rendered");
    const approveButtons = await within(list).findAllByRole("button", { name: /^Approve$/ });
    expect(approveButtons).toHaveLength(2);
    const [first, other] = approveButtons as [HTMLElement, HTMLElement];
    await userEvent.click(first);

    await waitFor(() => expect(first).toBeDisabled());
    expect(other).toBeEnabled();
  });

  // The bulk Approve is dimmed with nothing selected, and it must also BE disabled, so assistive
  // tech and the keyboard skip it rather than offering a dead button (#1659 map, Requests).
  it("disables bulk Approve until something is selected", async () => {
    stub({ proposals: [proposal, second] });
    render(<ApprovalQueue />);

    const selectAll = await screen.findByRole("checkbox", { name: "Select all for bulk approve" });
    const [list] = screen.getAllByRole("list");
    const bulkApprove = screen
      .getAllByRole("button", { name: /^Approve/ })
      .find((b) => !list?.contains(b)) as HTMLElement;
    expect(screen.getByText("None selected")).toBeInTheDocument();
    expect(bulkApprove).toBeDisabled();

    await userEvent.click(selectAll);
    expect(bulkApprove).toBeEnabled();
    expect(bulkApprove).toHaveAccessibleName("Approve 2");
  });

  // #1430: a failed decision used to surface in one banner above the whole queue, away from the
  // row it happened to. It must render on the row whose approve/dismiss actually failed, and must
  // not appear on an unrelated row rendered at the same time.
  it("renders an approve failure on its own row, not a page-level banner", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((url: string) => {
        if (url.includes(`/v1/proposals/${proposal.id}/approve`)) {
          return Promise.resolve(
            new Response(JSON.stringify({ title: "Couldn't approve", detail: "Try again" }), {
              status: 500,
              headers: { "content-type": "application/json" },
            }),
          );
        }
        if (url.includes("/v1/filler/pulls")) return Promise.resolve(jsonResponse({ pulls: [] }));
        if (url.includes("/v1/discovery/feedback")) return Promise.resolve(jsonResponse([]));
        if (url.endsWith("/outlook")) return Promise.resolve(jsonResponse(outlook({ state: "uncertain" })));
        if (url.includes("/v1/proposals"))
          return Promise.resolve(jsonResponse({ proposals: [proposal, second] }));
        return Promise.resolve(jsonResponse({}));
      }),
    );
    render(<ApprovalQueue />);

    const [list] = await screen.findAllByRole("list");
    if (!list) throw new Error("no proposal list rendered");
    const approveButtons = await within(list).findAllByRole("button", { name: /^Approve$/ });
    await userEvent.click(approveButtons[0] as HTMLElement);

    const failedRow = (await screen.findByText("90s action night")).closest("li");
    const otherRow = screen.getByText("Noir").closest("li");
    if (!failedRow || !otherRow) throw new Error("rows not found");

    expect(await within(failedRow).findByRole("alert")).toHaveTextContent("Couldn't approve");
    expect(within(otherRow).queryByRole("alert")).not.toBeInTheDocument();
  });

  it("renders a filler-pull decision failure on its own card, not on another pull's", async () => {
    const otherPull = { ...pull, id: "pull_2", title: "Retro ads" };
    vi.stubGlobal(
      "fetch",
      vi.fn((url: string) => {
        if (url.includes(`/v1/filler/pulls/${pull.id}/dismiss`)) {
          return Promise.resolve(
            new Response(JSON.stringify({ title: "Couldn't dismiss", detail: "Try again" }), {
              status: 500,
              headers: { "content-type": "application/json" },
            }),
          );
        }
        if (url.includes("/v1/filler/pulls"))
          return Promise.resolve(jsonResponse({ pulls: [pull, otherPull] }));
        if (url.includes("/v1/discovery/feedback")) return Promise.resolve(jsonResponse([]));
        if (url.includes("/v1/proposals")) return Promise.resolve(jsonResponse({ proposals: [] }));
        return Promise.resolve(jsonResponse({}));
      }),
    );
    render(<ApprovalQueue />);

    const notNowButtons = await screen.findAllByRole("button", { name: "Not now" });
    await userEvent.click(notNowButtons[0] as HTMLElement);

    const failedCard = (await screen.findByText("Top up the 1990s")).closest('div[class*="rounded-lg"]');
    const otherCard = screen.getByText("Retro ads").closest('div[class*="rounded-lg"]');
    if (!failedCard || !otherCard) throw new Error("pull cards not found");

    expect(await within(failedCard as HTMLElement).findByRole("alert")).toHaveTextContent("Couldn't dismiss");
    expect(within(otherCard as HTMLElement).queryByRole("alert")).not.toBeInTheDocument();
  });
});
