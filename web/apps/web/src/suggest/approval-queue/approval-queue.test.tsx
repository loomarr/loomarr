import type { EpisodeSelection } from "@loomarr/api/models/episodeSelection";
import { LoomarrProvider } from "@loomarr/design-system";
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
  // LoomarrProvider: the bulk groups and consequence dialog are @loomarr/ui components, which read
  // the design-system theme the app root provides.
  return ({ children }: { children: ReactNode }) => (
    <LoomarrProvider theme="dark">
      <QueryClientProvider client={client}>
        <TooltipProvider>{children}</TooltipProvider>
      </QueryClientProvider>
    </LoomarrProvider>
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
    rationale: "The Velvet Cartographer leads it",
    lineup: [
      { name: "The Velvet Cartographer", year: 1995, mediaType: "movie", tmdbId: 90000949, inLibrary: true },
    ],
    acquisitions: [
      { name: "Kettle Kids", year: 1989, mediaType: "series", tvdbId: 90071663, inLibrary: false },
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
              { name: "Added Series", year: 1989, mediaType: "series", tmdbId: 90000456, inLibrary: false },
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
                  lineup: [
                    {
                      name: "The Velvet Cartographer",
                      mediaType: "movie",
                      tmdbId: 90000949,
                      inLibrary: true,
                    },
                  ],
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

// A row's own Approve, which carries that row's edit. The group head's bulk Approve never answers
// to the bare name — it says it approves the selection (#1904) — so the name alone finds the row's.
const rowApprove = async () => {
  const [button] = await screen.findAllByRole("button", { name: /^Approve$/ });
  if (!button?.closest('[role="listitem"]')) throw new Error("no row Approve rendered");
  return button;
};

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
    expect(screen.getByRole("checkbox", { name: "Include The Velvet Cartographer" })).toBeVisible();
    expect(approvals).toEqual([]);
  });

  it("mounts the edit surface from the queue", async () => {
    stubApi();
    render(<ApprovalQueue />);

    const toggle = await screen.findByRole("button", { name: /Review & edit picks/ });
    await userEvent.click(toggle);

    expect(screen.getByText("Titles")).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Include The Velvet Cartographer" })).toBeInTheDocument();
    expect(screen.getByLabelText("Note to the requester")).toBeInTheDocument();
  });

  // ⚠ Approving an untouched proposal must send the SAME body it always did. The handler maps a
  // body with no drops/adds/note to a nil edit, so an empty object is the pre-V25 behaviour
  // exactly; anything else would record "approved with modifications: none" — false.
  it("approving unmodified sends an empty body, exactly as before", async () => {
    const approvals = stubApi();
    render(<ApprovalQueue />);

    await userEvent.click(await rowApprove());

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
    await userEvent.click(screen.getByRole("checkbox", { name: "Include The Velvet Cartographer" }));
    await userEvent.type(screen.getByLabelText("Note to the requester"), "too violent");
    await userEvent.click(await rowApprove());

    await waitFor(() => expect(approvals).toHaveLength(1));
    expect(approvals[0]).toEqual({ drop: ["movie:tmdb:90000949"], note: "too violent" });
  });

  it("undoing every edit returns to the unmodified body", async () => {
    const approvals = stubApi();
    render(<ApprovalQueue />);

    await userEvent.click(await screen.findByRole("button", { name: /Review & edit picks/ }));
    await userEvent.click(screen.getByRole("checkbox", { name: "Include The Velvet Cartographer" }));
    await userEvent.click(screen.getByRole("checkbox", { name: "Include The Velvet Cartographer" }));
    await userEvent.click(await rowApprove());

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
    await userEvent.click(await rowApprove());
    await waitFor(() => expect(approvals).toHaveLength(1));
    expect(approvals[0]).toEqual({
      add: [{ name: "Added Series", mediaType: "series", inLibrary: false, year: 1989, tmdbId: 90000456 }],
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

    // Scoped to the rows: the group head carries its own bulk "Approve" button.
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

    const selectAll = await screen.findByRole("checkbox", { name: "Select all Channel requests" });
    const [list] = screen.getAllByRole("list");
    const bulkApprove = screen
      .getAllByRole("button", { name: /^Approve/ })
      .find((b) => !list?.contains(b)) as HTMLElement;
    expect(screen.getByText("None selected")).toBeInTheDocument();
    expect(bulkApprove).toBeDisabled();

    await userEvent.click(selectAll);
    expect(bulkApprove).toBeEnabled();
    expect(bulkApprove).toHaveAccessibleName("Approve 2 selected in channel requests");
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

    const failedRow = (await screen.findByText("90s action night")).closest<HTMLElement>('[role="listitem"]');
    const otherRow = screen.getByText("Noir").closest<HTMLElement>('[role="listitem"]');
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

    const failedCard = await screen.findByRole("group", { name: "Top up the 1990s" });
    const otherCard = screen.getByRole("group", { name: "Retro ads" });

    expect(await within(failedCard).findByRole("alert")).toHaveTextContent("Couldn't dismiss");
    expect(within(otherCard).queryByRole("alert")).not.toBeInTheDocument();
  });
});

// #1817 / mock #1876 — each group's bulk approve shows what it will do, confirms, then shows what
// it did, with a retry per failure. Every figure in the summary comes from the server.
describe("ApprovalQueue — bulk approve per group with a consequence summary", () => {
  const GiB = 1024 ** 3;
  const sized = {
    ...proposal,
    id: "p1",
    jobId: "job_1",
    proposal: {
      ...proposal.proposal,
      channelName: "Action Night",
      policy: { audience: { ceiling: "PG-13" } },
      lineup: [
        {
          name: "The Velvet Cartographer",
          mediaType: "movie",
          inLibrary: true,
          sizeBytes: 2 * GiB,
          sizeConfidence: "exact",
        },
      ],
    },
  };
  const unsized = {
    ...proposal,
    id: "p2",
    jobId: "job_2",
    proposal: { ...proposal.proposal, intent: { description: "Noir" } },
  };
  const pull = {
    id: "pull_1",
    title: "Top up the 1990s",
    reason: "Saturday Mornings falls back to bumpers.",
    proposedBy: "filler-sweep",
    status: "pending",
    estimateClips: 52,
    candidateCount: 12,
    rejected: [],
    sources: [],
    createdAt: "2026-08-01T12:00:00Z",
    plan: [
      {
        sourceId: "classic",
        candidateId: "c1",
        tag: "archive",
        name: "Classic ads",
        why: "",
        estimateClips: 40,
        dropped: false,
      },
      {
        sourceId: "psa",
        candidateId: "c2",
        tag: "archive",
        name: "PSAs",
        why: "",
        estimateClips: 12,
        dropped: false,
      },
    ],
  };
  const otherPull = { ...pull, id: "pull_2", title: "Retro ads" };

  // Records every bulk body; `answer` decides each bulk call's per-item results.
  const stub = (opts: {
    channels?: unknown[];
    answer?: (ids: string[]) => { id: string; ok: boolean; error?: string; enqueued?: number }[];
  }) => {
    const bulkCalls: { url: string; ids: string[] }[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn((url: string, init?: RequestInit) => {
        if (url.endsWith("/v1/proposals/approve") || url.endsWith("/v1/filler/pulls/approve")) {
          const { ids } = JSON.parse(init?.body as string) as { ids: string[] };
          bulkCalls.push({ url, ids });
          const results = (opts.answer ?? ((xs) => xs.map((id) => ({ id, ok: true, enqueued: 1 }))))(ids);
          return Promise.resolve(jsonResponse({ approved: results.filter((r) => r.ok).length, results }));
        }
        if (url.includes("/v1/channels"))
          return Promise.resolve(jsonResponse({ channels: opts.channels ?? [] }));
        if (url.includes("/v1/filler/pulls"))
          return Promise.resolve(jsonResponse({ pulls: [pull, otherPull] }));
        if (url.includes("/v1/discovery/feedback")) return Promise.resolve(jsonResponse([]));
        if (url.endsWith("/outlook")) return Promise.resolve(jsonResponse(outlook({ state: "uncertain" })));
        if (url.includes("/v1/proposals"))
          return Promise.resolve(jsonResponse({ proposals: [sized, unsized] }));
        return Promise.resolve(jsonResponse({}));
      }),
    );
    return bulkCalls;
  };

  const group = (name: string) => screen.findByRole("group", { name });
  const groupApprove = (g: HTMLElement) =>
    within(g)
      .getAllByRole("button", { name: /^Approve/ })
      .find((b) => !b.closest('[role="listitem"]')) as HTMLElement;

  it("keeps the two groups' selections and approvals apart", async () => {
    stub({});
    render(<ApprovalQueue />);

    const channels = await group("Channel requests");
    const filler = await group("Filler downloads");
    await userEvent.click(
      within(channels).getByRole("checkbox", { name: "Select 90s action night for bulk approve" }),
    );

    expect(within(channels).getByText("1 selected")).toBeInTheDocument();
    expect(within(filler).getByText("None selected")).toBeInTheDocument();
    expect(groupApprove(filler)).toBeDisabled();
  });

  it("summarises before approving and sends nothing until confirmed", async () => {
    const bulkCalls = stub({});
    render(<ApprovalQueue />);

    const channels = await group("Channel requests");
    await userEvent.click(within(channels).getByRole("checkbox", { name: "Select all Channel requests" }));
    await waitFor(() => expect(groupApprove(channels)).toBeEnabled());
    await userEvent.click(groupApprove(channels));

    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("Approve 2 channel requests?")).toBeInTheDocument();
    expect(within(dialog).getByText("2 titles · 1 to download · up to PG-13")).toBeInTheDocument();
    // An owned movie has its exact size; the series still to acquire has none, and says so.
    expect(within(dialog).getByText("Size: 2.0 GB for 1 of 2 titles · 1 unknown")).toBeInTheDocument();
    expect(within(dialog).getByText("Size: estimate unavailable")).toBeInTheDocument();
    expect(within(dialog).getByText('Becomes a new channel named "Action Night"')).toBeInTheDocument();
    expect(within(dialog).getByText("2 titles will start downloading.")).toBeInTheDocument();
    expect(within(dialog).getByText("Size: 2.0 GB for 1 of 4 titles · 3 unknown")).toBeInTheDocument();
    expect(bulkCalls).toHaveLength(0);

    // Cancel approves nothing and keeps the selection.
    await userEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(bulkCalls).toHaveLength(0);
    expect(within(channels).getByText("2 selected")).toBeInTheDocument();
  });

  it("says it updates the channel a revised request already has", async () => {
    stub({ channels: [{ id: "ch_1", name: "Friday Fights", intentRef: "job_1" }] });
    render(<ApprovalQueue />);

    const channels = await group("Channel requests");
    await userEvent.click(
      within(channels).getByRole("checkbox", { name: "Select 90s action night for bulk approve" }),
    );
    await waitFor(() => expect(groupApprove(channels)).toBeEnabled());
    await userEvent.click(groupApprove(channels));

    expect(await screen.findByText('Updates the channel "Friday Fights"')).toBeInTheDocument();
    expect(screen.queryByText(/Becomes a new channel/)).not.toBeInTheDocument();
  });

  it("shows a partial failure per item and retries only the failed ids", async () => {
    let calls = 0;
    const bulkCalls = stub({
      answer: (ids) =>
        ids.map((id) =>
          calls++ === 1 && id === "p2"
            ? { id, ok: false, error: "Couldn't save the approval.", status: 500, retryable: true }
            : { id, ok: true, enqueued: 1 },
        ),
    });
    render(<ApprovalQueue />);

    const channels = await group("Channel requests");
    await userEvent.click(within(channels).getByRole("checkbox", { name: "Select all Channel requests" }));
    await waitFor(() => expect(groupApprove(channels)).toBeEnabled());
    await userEvent.click(groupApprove(channels));
    await userEvent.click(
      within(await screen.findByRole("dialog")).getByRole("button", { name: "Approve 2" }),
    );

    const dialog = await screen.findByRole("dialog");
    expect(await within(dialog).findByText("Approved 1 of 2")).toBeInTheDocument();
    expect(within(dialog).getByText('1 title enqueued · channel "Action Night" created')).toBeInTheDocument();
    expect(within(dialog).getByText("Couldn't save the approval.")).toBeInTheDocument();
    expect(bulkCalls[0]).toEqual({ url: "/v1/proposals/approve", ids: ["p1", "p2"] });

    await userEvent.click(within(dialog).getByRole("button", { name: "Retry Noir" }));
    expect(await within(dialog).findByText("Approved 2 of 2")).toBeInTheDocument();
    expect(bulkCalls[1]).toEqual({ url: "/v1/proposals/approve", ids: ["p2"] });

    // Done closes and clears that group's selection.
    await userEvent.click(within(dialog).getByRole("button", { name: "Done" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(within(channels).getByText("None selected")).toBeInTheDocument();
  });

  it("states a filler download's missing size and channel as unavailable, and its exact clip count after", async () => {
    const bulkCalls = stub({ answer: (ids) => ids.map((id) => ({ id, ok: true, enqueued: 12 })) });
    render(<ApprovalQueue />);

    const filler = await group("Filler downloads");
    expect(within(filler).getByText("Requested by Loomarr")).toBeInTheDocument();
    await userEvent.click(
      within(filler).getByRole("checkbox", { name: "Select Top up the 1990s for bulk approve" }),
    );
    await userEvent.click(groupApprove(filler));

    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("Approve 1 filler download?")).toBeInTheDocument();
    expect(
      within(dialog).getByText("~52 clips from 2 sources (estimate — may yield fewer once fetched)"),
    ).toBeInTheDocument();
    expect(within(dialog).getByText("Channel: unavailable")).toBeInTheDocument();
    expect(within(dialog).getAllByText("Size: estimate unavailable")).toHaveLength(2);
    expect(within(dialog).queryByText(/0 B|General catalog variety/)).not.toBeInTheDocument();

    await userEvent.click(within(dialog).getByRole("button", { name: "Approve 1" }));
    expect(await within(dialog).findByText("12 clips queued")).toBeInTheDocument();
    expect(bulkCalls[0]).toEqual({ url: "/v1/filler/pulls/approve", ids: ["pull_1"] });
  });

  // Bulk carries ids only, so it would silently drop a left-out row or a note.
  it("keeps an edited pull out of bulk and says why", async () => {
    stub({});
    render(<ApprovalQueue />);

    const filler = await group("Filler downloads");
    await userEvent.click(
      within(filler).getByRole("checkbox", { name: "Select Top up the 1990s for bulk approve" }),
    );
    await userEvent.click(
      within(filler).getAllByRole("button", { name: "Leave out Classic ads" })[0] as HTMLElement,
    );

    expect(
      within(filler).queryByRole("checkbox", { name: "Select Top up the 1990s for bulk approve" }),
    ).not.toBeInTheDocument();
    expect(
      within(filler).getByText("Excluded from bulk — approve individually so the edit is kept."),
    ).toBeInTheDocument();
    expect(within(filler).getByText("None selected")).toBeInTheDocument();
  });
});
