import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RouterHarness } from "@/test/story-utils";
import { ChannelSuggestPanel } from "./channel-suggest-panel";

// The panel reuses the whole Suggest flow (useSuggestionRun → GenerationProgress →
// ProposalReview), so its test mirrors suggest-workspace's harness: an admin auth/me, a POST
// /v1/proposals that returns a jobId, a /v1/proposals list that yields a submitted proposal
// matched on that jobId, and a stubbed EventSource (jsdom has none — the phases ride SSE, the
// proposal rides the list). The panel needs a router (it navigates on approve) + a query
// client + the events provider is absent in isolation (the listener is then a no-op, exactly
// as suggest-workspace notes).
const ADMIN = { id: "u1", name: "Ada", role: "admin", autoApprove: true, disabled: false, quota: 0 };
const MEMBER = { ...ADMIN, role: "member" };

const PROPOSAL = {
  id: "p-1",
  jobId: "job-1",
  status: "submitted",
  proposal: {
    intent: { description: "80s teen comedies" },
    lineup: [
      { mediaType: "movie", tmdbId: 9377, name: "Ferris Bueller's Day Off", year: 1986, inLibrary: true },
    ],
    acquisitions: [],
    rationale: "Grounded against your library.",
  },
};

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const stubFetch = (opts: { proposals?: unknown[]; me?: unknown; approveBody?: unknown } = {}) => {
  const mock = vi.fn((url: string, init?: RequestInit) => {
    const u = String(url);
    const method = String(init?.method ?? "GET");
    if (u.includes("/v1/auth/me")) return Promise.resolve(json(opts.me ?? ADMIN));
    // Approve — returns the created channel's id (what the panel navigates to).
    if (u.includes("/approve") && method === "POST") {
      return Promise.resolve(json(opts.approveBody ?? { channelId: "ch_new123" }));
    }
    if (u.includes("/v1/proposals") && method === "POST") return Promise.resolve(json({ jobId: "job-1" }));
    if (u.includes("/v1/proposals") || u.includes("/v1/proposals")) {
      return Promise.resolve(json({ proposals: opts.proposals ?? [] }));
    }
    return Promise.resolve(json({}));
  });
  vi.stubGlobal("fetch", mock);
  vi.stubGlobal(
    "EventSource",
    class {
      addEventListener() {}
      close() {}
    },
  );
  return mock;
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

afterEach(() => vi.restoreAllMocks());

describe("ChannelSuggestPanel", () => {
  it("submits the typed intent to start a run", async () => {
    const user = userEvent.setup();
    const fetchMock = stubFetch();
    renderPanel(() => {});

    await user.type(await screen.findByLabelText("Channel intent"), "80s teen comedies");
    await user.click(screen.getByRole("button", { name: /suggest a lineup/i }));

    await waitFor(() => {
      const posted = fetchMock.mock.calls.find(
        ([u, init]) => String(u).includes("/v1/proposals") && String(init?.method) === "POST",
      );
      expect(posted).toBeTruthy();
      expect(JSON.parse(String(posted?.[1]?.body))).toMatchObject({ description: "80s teen comedies" });
    });
  });

  // Moved here when `/suggest` folded into the Guide header (§12) and its route-level suite
  // went away. Worth keeping as its own case: `runtimeTargetMin` was in the shared schema and
  // consumed by the scorer for a long time with NO way to set it, so this pins that the
  // constraints disclosure actually reaches the wire — under the wire's field names.
  it("submits the constraints behind the disclosure, under the wire's field names", async () => {
    const user = userEvent.setup();
    const fetchMock = stubFetch();
    renderPanel(() => {});

    await user.type(await screen.findByLabelText("Channel intent"), "90s action movies");
    await user.click(screen.getByRole("button", { name: /add constraints/i }));
    await user.type(screen.getByLabelText(/target runtime/i), "180");
    await user.click(screen.getByRole("button", { name: /suggest a lineup/i }));

    await waitFor(() => {
      const posted = fetchMock.mock.calls.find(
        ([u, init]) => String(u).includes("/v1/proposals") && String(init?.method) === "POST",
      );
      expect(posted).toBeTruthy();
      expect(JSON.parse(String(posted?.[1]?.body))).toMatchObject({
        description: "90s action movies",
        runtimeTargetMin: 180,
      });
    });
  });

  it("shows the grounded proposal inline once the run produces one", async () => {
    const user = userEvent.setup();
    stubFetch({ proposals: [PROPOSAL] });
    renderPanel(() => {});

    await user.type(await screen.findByLabelText("Channel intent"), "80s teen comedies");
    await user.click(screen.getByRole("button", { name: /suggest a lineup/i }));

    // The reused ProposalReview renders the lineup — no navigation away from the panel.
    expect(await screen.findByText("Ferris Bueller's Day Off")).toBeInTheDocument();
  });

  it("approving hands the new channel id to onCreated (the list navigates to it)", async () => {
    const user = userEvent.setup();
    stubFetch({ proposals: [PROPOSAL], approveBody: { channelId: "ch_new123" } });
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
    const fetchMock = stubFetch({ proposals: [PROPOSAL], me: MEMBER });
    const onCreated = vi.fn();
    renderPanel(onCreated);

    await user.type(await screen.findByLabelText("Channel intent"), "80s teen comedies");
    await user.click(screen.getByRole("button", { name: /suggest a lineup/i }));
    await user.click(await screen.findByRole("button", { name: /approve/i }));

    // No approve POST, no navigation — the control is wired to nothing for a member.
    expect(fetchMock.mock.calls.some(([u]) => String(u).includes("/approve"))).toBe(false);
    expect(onCreated).not.toHaveBeenCalled();
  });
});
