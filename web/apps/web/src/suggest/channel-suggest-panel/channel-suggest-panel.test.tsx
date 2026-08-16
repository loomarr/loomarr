import type { ApproveOutputBody, MeBody, ProposalDTO } from "@loomarr/api";
import {
  getApproveProposalMockHandler,
  getFillerPoolMockHandler,
  getListProposalsMockHandler,
  getMeMockHandler,
  getSubmitProposalMockHandler,
} from "@loomarr/api/msw";
import { CHANNEL_TEMPLATES } from "@loomarr/core/templates";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { me } from "@/test/fixtures/users";
import { server } from "@/test/msw/server";
import { RouterHarness } from "@/test/story-utils";
import { ChannelSuggestPanel } from "./channel-suggest-panel";

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
    scores: { themeFit: 0.9, availabilityRatio: 1, eraBalance: 0.7, overall: 0.85 },
    rationale: "Grounded against your library.",
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
  opts: {
    proposals?: ProposalDTO[];
    me?: MeBody;
    approveBody?: ApproveOutputBody;
    fillerEligible?: number;
  } = {},
) => {
  const approvals: string[] = [];
  const submissions: unknown[] = [];
  const fillerEligible = opts.fillerEligible ?? 0;

  server.use(
    getMeMockHandler(opts.me ?? ADMIN),
    getFillerPoolMockHandler({
      channels: [],
      clips: fillerEligible,
      commercials: fillerEligible,
      eligible: fillerEligible,
      untagged: 0,
    }),
    // Approve — returns the created channel's id (what the panel navigates to).
    getApproveProposalMockHandler(({ params }) => {
      approvals.push(String(params.id));
      return opts.approveBody ?? { channelId: "ch_new123", enqueued: 0, status: "approved" };
    }),
    getSubmitProposalMockHandler(async ({ request }) => {
      submissions.push(await request.json());
      return { jobId: "job-1" };
    }),
    getListProposalsMockHandler({ proposals: opts.proposals ?? [] }),
  );

  return { approvals, submissions };
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

  it.each(CHANNEL_TEMPLATES)("submits the complete $label preset intent", async ({ label, intent }) => {
    const user = userEvent.setup();
    const { submissions } = stubSuggest();
    renderPanel(() => {});

    await user.click(await screen.findByRole("button", { name: label }));
    await user.click(screen.getByRole("button", { name: /suggest a lineup/i }));

    await waitFor(() => {
      expect(submissions).toEqual([intent]);
    });
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

  it("keeps approval available when no filler is ready and explains back-to-back playout", async () => {
    const user = userEvent.setup();
    stubSuggest({ proposals: [PROPOSAL], fillerEligible: 0 });
    renderPanel(() => {});

    await user.type(await screen.findByLabelText("Channel intent"), "80s teen comedies");
    await user.click(screen.getByRole("button", { name: /suggest a lineup/i }));

    expect(
      await screen.findByText(
        "No break-ready filler yet. This channel will play programs back-to-back, and you can add filler later.",
      ),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /approve/i })).toBeEnabled();
  });

  it("reports ready commercial filler without blocking approval", async () => {
    const user = userEvent.setup();
    stubSuggest({ proposals: [PROPOSAL], fillerEligible: 12 });
    renderPanel(() => {});

    await user.type(await screen.findByLabelText("Channel intent"), "80s teen comedies");
    await user.click(screen.getByRole("button", { name: /suggest a lineup/i }));

    expect(
      await screen.findByText("Commercial filler is available and will be tuned after creation."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /approve/i })).toBeEnabled();
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
    await user.click(await screen.findByRole("button", { name: /approve/i }));

    await waitFor(() => expect(onCreated).toHaveBeenCalledWith("ch_new123"));
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
    await user.click(await screen.findByRole("button", { name: /approve/i }));

    // No approve POST, no navigation — the control is wired to nothing for a member.
    // ⚠ `approvals` is fed only by `POST /v1/proposals/:id/approve` — the per-proposal route the
    // panel would call. The old `includes("/approve")` would also have matched the BULK route.
    expect(approvals).toEqual([]);
    expect(onCreated).not.toHaveBeenCalled();
  });
});
