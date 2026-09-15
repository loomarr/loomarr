import type { ApproveOutputBody, MeBody, ProposalDTO } from "@loomarr/api";
import {
  getApproveProposalMockHandler,
  getGetProposalJobMockHandler,
  getGetProposalOutlookMockHandler,
  getMeMockHandler,
  getReviseProposalJobMockHandler,
  getSubmitProposalMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { outlook } from "@/test/fixtures/outlook";
import { me } from "@/test/fixtures/users";
import { server } from "@/test/msw/server";
import { RouterHarness } from "@/test/story-utils";
import type { SuggestionRun } from "../use-suggestion-run/use-suggestion-run.type";
import { ChannelSuggestPanel } from "./channel-suggest-panel";

// The failed-run case is driven by a hook override: the failure arrives as an SSE `failed`
// phase, which the panel test's isolated (provider-less) render can't emit. useSuggestionRun's
// own test proves the phase→failed mapping; this proves the panel RENDERS that failure instead
// of silently dropping back to the describe form (the reported bug). Default: delegate to the
// REAL hook so every other test in this file keeps exercising the true flow through MSW.
let runOverride: SuggestionRun | undefined;
vi.mock("../use-suggestion-run", async (importActual) => {
  const actual = await importActual<typeof import("../use-suggestion-run")>();
  const useReal = actual.useSuggestionRun;
  // biome-ignore lint/correctness/useHookAtTopLevel: mock replacement hook delegating to the real one, not a conditional component hook.
  return { useSuggestionRun: () => runOverride ?? useReal() };
});

const failedRun = (over: Partial<SuggestionRun> = {}): SuggestionRun => ({
  phase: "failed",
  round: undefined,
  proposal: undefined,
  failure: {
    code: "generation_failed",
    message: "Loomarr couldn't generate this channel.",
    reason: "provider_unavailable",
    recoveryAction: "retry_later",
    guidance: "Try again later. If this continues, ask an administrator to check AI.",
  },
  actions: ["retry", "check_ai"],
  isRunning: false,
  failed: true,
  error: undefined,
  start: vi.fn(),
  revise: vi.fn(),
  retry: vi.fn(),
  reset: vi.fn(),
  ...over,
});

// The panel reuses the whole Suggest flow (useSuggestionRun → GenerationProgress →
// ProposalReview), so its test mirrors suggest-workspace's harness: an admin auth/me, a POST
// /v1/proposals that returns a jobId, a /v1/proposals list that yields a submitted proposal
// matched on that jobId, and a stubbed EventSource (jsdom has none — the phases ride SSE, the
// proposal rides the list). The panel needs a router (it navigates on approve) + a query
// client + the events provider is absent in isolation (the listener is then a no-op, exactly
// as suggest-workspace notes).
// ⚠ `local` is REQUIRED on MeBody and this fixture omitted it.
const ADMIN: MeBody = me();
const MEMBER: MeBody = me({ role: "member" });

const PROPOSAL: ProposalDTO = {
  id: "p-1",
  jobId: "job-1",
  status: "submitted",
  proposal: {
    intent: { description: "80s teen comedies" },
    lineup: [
      { mediaType: "movie", tmdbId: 9377, name: "Ferris Bueller's Day Off", year: 1986, inLibrary: true },
    ],
    acquisitions: [],
    // ⚠ `alternates` and `scores` are REQUIRED on Proposal and this fixture had neither. The
    // panel renders ProposalReview, which reads `scores` for the fit summary — so the review was
    // being exercised against a proposal the server could not have produced.
    alternates: [],
    scores: {
      version: 1,
      themeFit: 1,
      availabilityRatio: 1,
      eraBalance: null,
      theme: {
        status: "supported",
        basis: "qualifiers",
        assessedItems: 2,
        unknownItems: 0,
        qualifiers: [{ term: "action", supportedItems: 2 }],
      },
      era: { status: "not_requested", assessedItems: 0, matchingItems: 0, unknownItems: 0 },
    },
    rationale: "Grounded against your library.",
    trace: { version: 1, surfacedTotal: 0, recordedTotal: 0, truncated: false, candidates: [] },
  },
};

// ⚠ `u.includes("/v1/proposals") || u.includes("/v1/proposals")` — the SAME condition twice, the
// second branch unreachable. It is the second duplicated-`/v1/proposals` branch this migration has
// found (`test/reachability` had the other), and neither could have been noticed: dead code in a
// stub produces no symptom at all.
//
// ⚠ `u.includes("/approve")` also matches `POST /v1/proposals/approve`, the BULK route. The
// member-gate test below asserts NO approve call fires — an assertion whose whole value depends on
// naming the right endpoint.
const stubSuggest = (
  opts: { proposals?: ProposalDTO[]; me?: MeBody; approveBody?: ApproveOutputBody } = {},
) => {
  const approvals: { id: string; edit: unknown }[] = [];
  const submissions: unknown[] = [];
  const revisions: unknown[] = [];

  server.use(
    getMeMockHandler(opts.me ?? ADMIN),
    // Approve — returns the created channel's id (what the panel navigates to).
    getApproveProposalMockHandler(async ({ params, request }) => {
      approvals.push({ id: String(params.id), edit: await request.json() });
      return opts.approveBody ?? { channelId: "ch_new123", enqueued: 0, status: "approved" };
    }),
    getSubmitProposalMockHandler(async ({ request }) => {
      submissions.push(await request.json());
      return { jobId: "job-1" };
    }),
    getReviseProposalJobMockHandler(async ({ params, request }) => {
      revisions.push(await request.json());
      return { jobId: String(params.jobId) };
    }),
    getGetProposalJobMockHandler(() => {
      const proposal = opts.proposals?.[0];
      return {
        version: 1,
        jobId: "job-1",
        milestone: proposal ? "awaiting_approval" : "generating",
        intent: { description: "80s teen comedies" },
        attempts: [
          {
            version: 1,
            number: 1,
            status: proposal ? "succeeded" : "running",
            startedAt: "2026-08-22T12:00:00Z",
          },
        ],
        proposal: proposal
          ? { id: proposal.id, status: proposal.status, proposal: proposal.proposal }
          : undefined,
        actions: proposal ? ["review", "edit"] : ["wait"],
        createdAt: "2026-08-22T12:00:00Z",
        updatedAt: "2026-08-22T12:00:00Z",
      };
    }),
  );

  return { approvals, submissions, revisions };
};

const renderPanel = (onCreated: (id: string) => void) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <RouterHarness
      content={
        <QueryClientProvider client={client}>
          <ChannelSuggestPanel onCreated={onCreated} />
        </QueryClientProvider>
      }
    />,
  );
};

describe("ChannelSuggestPanel", () => {
  beforeEach(() => {
    server.use(
      getGetProposalOutlookMockHandler(
        outlook({
          state: "uncertain",
          unknownTitles: 1,
          programs: 0,
          uniqueRuntimeMs: 0,
          firstRepeatMs: null,
        }),
      ),
    );
    runOverride = undefined; // default every test back to the real hook
    window.sessionStorage.clear();
  });

  it("submits the typed intent to start a run", async () => {
    const user = userEvent.setup();
    const { submissions } = stubSuggest();
    renderPanel(() => {});

    await user.type(await screen.findByLabelText("Channel intent"), "80s teen comedies");
    await user.click(screen.getByRole("button", { name: /suggest a lineup/i }));

    await waitFor(() => {
      expect(submissions).toHaveLength(1);
      expect(submissions[0]).toMatchObject({ description: "80s teen comedies" });
    });
  });

  it("preserves the intent and links AI settings when submission is not configured", async () => {
    const user = userEvent.setup();
    stubSuggest();
    server.use(
      http.post("*/v1/proposals", () =>
        HttpResponse.json(
          {
            type: "feature_not_configured",
            title: "AI isn't set up",
            detail: "Connect an AI provider in Settings → AI to build channels from a sentence.",
          },
          { status: 409 },
        ),
      ),
    );
    renderPanel(() => {});

    const intent = await screen.findByLabelText("Channel intent");
    await user.type(intent, "Saturday morning cartoons");
    await user.click(screen.getByRole("button", { name: /suggest a lineup/i }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/connect a provider.*choose a lineup model/i);
    expect(screen.getByRole("link", { name: /set up ai/i })).toHaveAttribute("href", "/settings/ai");
    expect(intent).toHaveValue("Saturday morning cartoons");
  });

  it("keeps configured AI truthful and routes a missing grounding key to TMDB", async () => {
    const user = userEvent.setup();
    stubSuggest();
    server.use(
      http.post("*/v1/proposals", () =>
        HttpResponse.json(
          {
            type: "grounding_not_configured",
            title: "TMDB is needed for channel suggestions",
            detail:
              "Connect TMDB in Settings → Connections so Loomarr can match this channel description to real titles.",
          },
          { status: 409 },
        ),
      ),
    );
    const first = renderPanel(() => {});

    await user.type(await screen.findByLabelText("Channel intent"), "Saturday morning cartoons");
    await user.click(screen.getByRole("button", { name: /suggest a lineup/i }));

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/AI is connected.*TMDB/i);
    expect(alert).not.toHaveTextContent(/AI isn't set up|Connect AI/i);
    expect(screen.getByRole("link", { name: /connect TMDB/i })).toHaveAttribute(
      "href",
      "/settings/connections?focus=tmdb",
    );

    first.unmount();
    renderPanel(() => {});
    expect(await screen.findByLabelText("Channel intent")).toHaveValue("Saturday morning cartoons");
  });

  it("gives members administrator guidance instead of an unusable settings link", async () => {
    const user = userEvent.setup();
    stubSuggest({ me: MEMBER });
    server.use(
      http.post("*/v1/proposals", () =>
        HttpResponse.json(
          {
            type: "grounding_not_configured",
            title: "TMDB is needed for channel suggestions",
            detail: "Connect TMDB in Settings → Connections.",
          },
          { status: 409 },
        ),
      ),
    );
    renderPanel(() => {});

    await user.type(await screen.findByLabelText("Channel intent"), "Saturday morning cartoons");
    await user.click(screen.getByRole("button", { name: /suggest a lineup/i }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/administrator needs to connect TMDB/i);
    expect(screen.queryByRole("link", { name: /connect TMDB/i })).not.toBeInTheDocument();
  });

  // Moved here when `/suggest` folded into the Guide header (§12) and its route-level suite
  // went away. Worth keeping as its own case: `runtimeTargetMin` was in the shared schema and
  // consumed by the scorer for a long time with NO way to set it, so this pins that the
  // constraints disclosure actually reaches the wire — under the wire's field names.
  it("submits the constraints behind the disclosure, under the wire's field names", async () => {
    const user = userEvent.setup();
    const { submissions } = stubSuggest();
    renderPanel(() => {});

    await user.type(await screen.findByLabelText("Channel intent"), "90s action movies");
    await user.click(screen.getByRole("button", { name: /add constraints/i }));
    await user.type(screen.getByLabelText(/target runtime/i), "180");
    await user.click(screen.getByRole("button", { name: /suggest a lineup/i }));

    await waitFor(() => {
      expect(submissions).toHaveLength(1);
      expect(submissions[0]).toMatchObject({
        description: "90s action movies",
        runtimeTargetMin: 180,
      });
    });
  });

  it("shows the grounded proposal inline once the run produces one", async () => {
    const user = userEvent.setup();
    stubSuggest({ proposals: [PROPOSAL] });
    renderPanel(() => {});

    await user.type(await screen.findByLabelText("Channel intent"), "80s teen comedies");
    await user.click(screen.getByRole("button", { name: /suggest a lineup/i }));

    // The reused ProposalReview renders the lineup — no navigation away from the panel.
    expect(await screen.findByText("Ferris Bueller's Day Off")).toBeInTheDocument();
  });

  it("keeps title selection usable when a live response has no alternate list", async () => {
    const user = userEvent.setup();
    stubSuggest({ proposals: [PROPOSAL] });
    server.use(
      http.get("*/v1/proposal-jobs/job-1", () =>
        HttpResponse.json({
          version: 1,
          jobId: "job-1",
          milestone: "awaiting_approval",
          intent: PROPOSAL.proposal.intent,
          attempts: [],
          proposal: { ...PROPOSAL, proposal: { ...PROPOSAL.proposal, alternates: null } },
          actions: ["review", "edit"],
          createdAt: "2026-09-15T12:00:00Z",
          updatedAt: "2026-09-15T12:00:00Z",
        }),
      ),
    );
    renderPanel(() => {});
    await user.type(await screen.findByLabelText("Channel intent"), "80s teen comedies");
    await user.click(screen.getByRole("button", { name: /suggest a lineup/i }));
    await user.click(await screen.findByRole("checkbox", { name: "Include Ferris Bueller's Day Off" }));
    await user.click(screen.getByText("Suggestion details"));
    expect(
      screen.getByText("You changed the title list. Check any titles you added against your brief."),
    ).toBeVisible();
    expect(screen.getByRole("button", { name: "Create channel" })).toBeDisabled();
  });

  it("revises a landed brief on the same Job without leaving the current review", async () => {
    const user = userEvent.setup();
    const { approvals, submissions, revisions } = stubSuggest({ proposals: [PROPOSAL] });
    renderPanel(() => {});
    await user.type(await screen.findByLabelText("Channel intent"), "80s teen comedies");
    await user.click(screen.getByRole("button", { name: /suggest a lineup/i }));
    await user.click(await screen.findByRole("button", { name: "Edit brief" }));
    expect(screen.getByText("Ferris Bueller's Day Off")).toBeVisible();
    expect(screen.getByRole("heading", { name: "Your new channel" })).toBeVisible();
    expect(screen.getByLabelText("Channel brief")).toHaveValue("80s teen comedies");
    expect(screen.queryByLabelText("Channel intent")).not.toBeInTheDocument();
    expect(approvals).toEqual([]);
    expect(submissions).toHaveLength(1);

    await user.clear(screen.getByLabelText("Channel brief"));
    await user.type(screen.getByLabelText("Channel brief"), "80s teen comedies with more variety");
    await user.click(screen.getByRole("button", { name: "Update suggestions" }));

    await waitFor(() => expect(revisions).toEqual([{ description: "80s teen comedies with more variety" }]));
    expect(submissions).toHaveLength(1);
    expect(screen.queryByLabelText("Channel intent")).not.toBeInTheDocument();
    expect(screen.getByText("Ferris Bueller's Day Off")).toBeVisible();
  });

  it("keeps the one add-title action inside the current review", async () => {
    const user = userEvent.setup();
    const { approvals } = stubSuggest({ proposals: [PROPOSAL] });
    server.use(getGetProposalOutlookMockHandler(outlook({ thin: true })));
    renderPanel(() => {});
    await user.type(await screen.findByLabelText("Channel intent"), "80s teen comedies");
    await user.click(screen.getByRole("button", { name: /suggest a lineup/i }));
    expect(await screen.findByText(/Short lineup:/)).toBeVisible();
    expect(screen.queryByRole("button", { name: "Add more variety" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Add title" }));
    expect(screen.getByPlaceholderText("Search for a movie or show…")).toBeVisible();
    expect(screen.getByRole("heading", { name: "Your new channel" })).toBeVisible();
    expect(approvals).toEqual([]);
  });

  it("explains an auto-approved result without offering the misleading Start over action", async () => {
    const user = userEvent.setup();
    const { approvals } = stubSuggest({ proposals: [{ ...PROPOSAL, status: "approved" }] });
    const onCreated = vi.fn();
    renderPanel(onCreated);

    await user.type(await screen.findByLabelText("Channel intent"), "80s teen comedies");
    await user.click(screen.getByRole("button", { name: /suggest a lineup/i }));

    expect(await screen.findByText("Ferris Bueller's Day Off")).toBeInTheDocument();
    expect(screen.getByText(/automatically approved/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /start over/i })).not.toBeInTheDocument();
    expect(approvals).toEqual([]);
    expect(onCreated).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: /create another/i }));

    expect(await screen.findByLabelText("Channel intent")).toHaveValue("");
    expect(approvals).toEqual([]);
    expect(onCreated).not.toHaveBeenCalled();
  });

  it("approving hands the new channel id to onCreated (the list navigates to it)", async () => {
    const user = userEvent.setup();
    stubSuggest({
      proposals: [PROPOSAL],
      approveBody: { channelId: "ch_new123", enqueued: 0, status: "approved" },
    });
    const onCreated = vi.fn();
    renderPanel(onCreated);

    await user.type(await screen.findByLabelText("Channel intent"), "80s teen comedies");
    await user.click(screen.getByRole("button", { name: /suggest a lineup/i }));
    await user.click(await screen.findByRole("button", { name: /create channel/i }));

    await waitFor(() => expect(onCreated).toHaveBeenCalledWith("ch_new123"));
  });

  it("creates the channel with the reviewer's exact title choices", async () => {
    const user = userEvent.setup();
    const editable: ProposalDTO = {
      ...PROPOSAL,
      proposal: {
        ...PROPOSAL.proposal,
        acquisitions: [{ mediaType: "movie", tmdbId: 1701, name: "Con Air", year: 1997, inLibrary: false }],
      },
    };
    const { approvals } = stubSuggest({ proposals: [editable] });
    renderPanel(() => {});

    await user.type(await screen.findByLabelText("Channel intent"), "80s teen comedies");
    await user.click(screen.getByRole("button", { name: /suggest a lineup/i }));
    await user.click(await screen.findByRole("checkbox", { name: "Include Ferris Bueller's Day Off" }));
    await user.click(screen.getByRole("button", { name: "Create channel" }));

    await waitFor(() => expect(approvals).toEqual([{ id: "p-1", edit: { drop: ["movie:tmdb:9377"] } }]));
  });

  it("normalizes Job-scoped title choices when a replacement landed while the review was closed", async () => {
    const replacement: ProposalDTO = {
      ...PROPOSAL,
      id: "p-2",
      proposal: {
        ...PROPOSAL.proposal,
        acquisitions: [{ mediaType: "movie", tmdbId: 603, name: "The Matrix", year: 1999, inLibrary: false }],
        alternates: [{ mediaType: "movie", tmdbId: 754, name: "Face/Off", year: 1997, inLibrary: false }],
      },
    };
    window.sessionStorage.setItem(
      "loomarr.proposalReviewEdit.job-1",
      JSON.stringify({
        drop: ["movie:tmdb:754"],
        add: [{ mediaType: "movie", tmdbId: 603, name: "The Matrix", year: 1999, inLibrary: false }],
      }),
    );
    runOverride = failedRun({
      jobId: "job-1",
      proposal: { id: replacement.id, status: replacement.status, proposal: replacement.proposal },
      failure: undefined,
      actions: ["review", "edit"],
      isRunning: false,
      failed: false,
    });
    stubSuggest();
    const view = renderPanel(() => {});

    expect(await screen.findByText("The Matrix")).toBeVisible();
    expect(screen.queryByText("Added by you")).not.toBeInTheDocument();
    await waitFor(() =>
      expect(JSON.parse(window.sessionStorage.getItem("loomarr.proposalReviewEdit.job-1") ?? "null")).toEqual(
        {
          drop: ["movie:tmdb:754"],
        },
      ),
    );
    view.unmount();
  });

  it("keeps a user-added title selected when the replacement returns it only as an alternate", async () => {
    const matrix = {
      mediaType: "movie",
      tmdbId: 603,
      name: "The Matrix",
      year: 1999,
      inLibrary: false,
    };
    window.sessionStorage.setItem("loomarr.proposalReviewEdit.job-1", JSON.stringify({ add: [matrix] }));
    runOverride = failedRun({
      jobId: "job-1",
      proposal: {
        id: "p-2",
        status: "submitted",
        proposal: { ...PROPOSAL.proposal, alternates: [matrix] },
      },
      failure: undefined,
      actions: ["review", "edit"],
      isRunning: false,
      failed: false,
    });
    stubSuggest();
    const view = renderPanel(() => {});

    await waitFor(() =>
      expect(JSON.parse(window.sessionStorage.getItem("loomarr.proposalReviewEdit.job-1") ?? "null")).toEqual(
        {
          drop: ["movie:tmdb:603"],
          add: [matrix],
        },
      ),
    );
    expect(screen.getAllByText("The Matrix")).toHaveLength(1);
    expect(screen.getByText("Will be added")).toBeVisible();
    view.unmount();
  });

  it("a member's approve is inert — no approve call fires (approval is admin-only, §7)", async () => {
    const user = userEvent.setup();
    // ProposalReview renders the Approve button off the proposal STATUS (same as /suggest);
    // the gate is that a member's onApprove is undefined, so clicking it does nothing — and
    // the server would 403 anyway. Assert the panel never fires the approve POST for a member.
    const { approvals } = stubSuggest({ proposals: [PROPOSAL], me: MEMBER });
    const onCreated = vi.fn();
    renderPanel(onCreated);

    await user.type(await screen.findByLabelText("Channel intent"), "80s teen comedies");
    await user.click(screen.getByRole("button", { name: /suggest a lineup/i }));
    expect(await screen.findByText("Sent for approval")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /create channel/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Edit brief" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add title" })).not.toBeInTheDocument();

    // No approve POST, no navigation — a member receives a read-only review.
    // ⚠ `approvals` is fed only by `POST /v1/proposals/:id/approve` — the per-proposal route the
    // panel would call. The old `includes("/approve")` would also have matched the BULK route.
    expect(approvals).toEqual([]);
    expect(onCreated).not.toHaveBeenCalled();
  });

  it("keeps the current review visible and read-only while its revision runs", async () => {
    runOverride = failedRun({
      phase: "reasoning",
      proposal: { id: PROPOSAL.id, status: PROPOSAL.status, proposal: PROPOSAL.proposal },
      failure: undefined,
      actions: ["wait"],
      isRunning: true,
      failed: false,
    });
    stubSuggest();
    renderPanel(() => {});

    expect(await screen.findByText("Ferris Bueller's Day Off")).toBeVisible();
    expect(screen.getByText(/Updating suggestions/)).toBeVisible();
    expect(await screen.findByRole("button", { name: "Create channel" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Edit brief" })).toBeDisabled();
  });

  it("restores the approvable review with local guidance when a revision fails", async () => {
    runOverride = failedRun({
      proposal: { id: PROPOSAL.id, status: PROPOSAL.status, proposal: PROPOSAL.proposal },
      actions: ["review", "edit", "retry"],
      failed: false,
    });
    stubSuggest();
    renderPanel(() => {});

    expect(await screen.findByText("Ferris Bueller's Day Off")).toBeVisible();
    expect(screen.getByRole("alert")).toHaveTextContent(/current lineup is unchanged/i);
    expect(screen.queryByText(/We couldn't finish this channel/i)).not.toBeInTheDocument();
    expect(await screen.findByRole("button", { name: "Create channel" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Edit brief" })).toBeEnabled();
  });

  // The reported bug: describe a channel, the job fails (e.g. no AI provider), and the panel
  // silently dropped back to an empty describe form — no error, no way to tell what happened.
  it("surfaces a failed run with a message and a retry instead of a silent empty form", async () => {
    const retry = vi.fn();
    runOverride = failedRun({ retry });
    stubSuggest();
    renderPanel(() => {});

    // The failure replaces progress with one recovery surface.
    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(screen.getByText(/couldn't generate this channel/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /check ai settings/i })).toHaveAttribute("href", "/settings/ai");
    // …and the describe form is NOT rendered underneath it (the silent-drop bug).
    expect(screen.queryByLabelText("Channel intent")).not.toBeInTheDocument();

    // Try again re-submits the preserved Intent through a fresh caller-owned Job.
    await userEvent.click(screen.getByRole("button", { name: /try again/i }));
    expect(retry).toHaveBeenCalled();
  });

  it("presents a catalog failure once with a direct retry action", async () => {
    runOverride = failedRun({
      failure: {
        code: "generation_failed",
        message: "Loomarr couldn't retrieve the catalog information needed for this request.",
        reason: "retrieval_unavailable",
        recoveryAction: "retry_later",
        guidance: "If this keeps happening, check the title sources in Connections.",
      },
      actions: ["retry"],
    });
    stubSuggest();
    renderPanel(() => {});

    expect(await screen.findByRole("alert")).toHaveTextContent(/We couldn't finish this channel/);
    expect(screen.getByText(/couldn't retrieve the catalog information/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Try again" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /check ai settings/i })).not.toBeInTheDocument();
  });

  it("does not blame the description when a grounded run cannot finish", async () => {
    const retry = vi.fn();
    runOverride = failedRun({
      retry,
      failure: {
        code: "budget_exhausted",
        message: "Loomarr found titles but couldn't finish the lineup.",
        reason: "provider_response_invalid",
        recoveryAction: "retry_later",
        guidance: "Try again. Your description is still here.",
      },
      actions: ["retry"],
    });
    stubSuggest();
    renderPanel(() => {});

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("We couldn't finish this channel");
    expect(alert).toHaveTextContent("Loomarr found titles but couldn't finish the lineup.");
    expect(alert).not.toHaveTextContent(/more specific|too many possible directions/i);
    expect(screen.queryByRole("button", { name: "Edit description" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(retry).toHaveBeenCalled();
  });

  it("does not expose discovery budgets or blame the description", async () => {
    const reset = vi.fn();
    runOverride = failedRun({
      reset,
      failure: {
        code: "budget_exhausted",
        message: "This request exceeded the bounded discovery budget.",
        reason: "discovery_budget_exhausted",
        recoveryAction: "simplify_request",
        guidance: "Simplify the request and try again.",
      },
      actions: ["edit", "retry"],
    });
    stubSuggest();
    renderPanel(() => {});

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("We couldn't finish the lineup");
    expect(alert).toHaveTextContent("Your description is still here. Try again, or edit it if you want to.");
    expect(alert).not.toHaveTextContent(/budget|more specific|too many possible directions/i);
    await userEvent.click(screen.getByRole("button", { name: "Edit description" }));
    expect(reset).toHaveBeenCalledWith(true);
  });
});
