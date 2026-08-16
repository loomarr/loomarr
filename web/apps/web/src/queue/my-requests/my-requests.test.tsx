import type { ProposalDTO, ProposalJobDTO } from "@loomarr/api";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as rtlRender, screen, waitFor } from "@testing-library/react";
import type { ReactElement, ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MyRequests } from "./my-requests";

const makeWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const render = (ui: ReactElement) => rtlRender(ui, { wrapper: makeWrapper() });

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });

const proposal = (over: Partial<ProposalDTO> = {}): ProposalDTO =>
  ({
    id: "p1",
    jobId: "j1",
    status: "submitted",
    proposal: { intent: { description: "90s action night" }, lineup: [], acquisitions: [] },
    ...over,
  }) as ProposalDTO;

const job = (over: Partial<ProposalJobDTO> = {}): ProposalJobDTO => ({
  jobId: "j1",
  status: "queued",
  intent: { description: "90s action night" },
  attempts: 0,
  createdAt: "2026-08-15T12:00:00Z",
  updatedAt: "2026-08-15T12:00:00Z",
  ...over,
});

// Records every Proposal Job read so the test can assert that ownership is resolved by the
// server's generated `mine` contract, not reconstructed from Proposal rows in the browser.
const stubProposalJobs = (jobs: ProposalJobDTO[]) => {
  const urls: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => {
      if (typeof url === "string" && url.includes("/v1/proposal-jobs")) {
        urls.push(url);
        return Promise.resolve(jsonResponse({ proposalJobs: jobs }));
      }
      return Promise.resolve(jsonResponse({}));
    }),
  );
  return urls;
};

afterEach(() => vi.unstubAllGlobals());

describe("MyRequests", () => {
  it("renders a caller-owned Proposal Job before a Proposal exists", async () => {
    stubProposalJobs([job()]);
    render(<MyRequests />);

    expect(await screen.findByText("My requests")).toBeInTheDocument();
    expect(screen.getByText("90s action night")).toBeInTheDocument();
    expect(screen.getByText("Queued")).toBeInTheDocument();
  });

  it("uses one authoritative caller-scoped Proposal Job list", async () => {
    const urls = stubProposalJobs([job()]);
    render(<MyRequests />);

    await screen.findByText("My requests");
    expect(urls).toHaveLength(1);
    expect(urls[0]).toContain("/v1/proposal-jobs");
    expect(urls[0]).toContain("mine=true");
    expect(urls[0]).not.toContain("status=");
  });

  it("shows queued, running, failed, and every done Proposal decision", async () => {
    stubProposalJobs([
      job({ jobId: "queued", intent: { description: "Queued channel" } }),
      job({ jobId: "running", status: "running", attempts: 1, intent: { description: "Running channel" } }),
      job({
        jobId: "failed",
        status: "failed",
        attempts: 1,
        intent: { description: "Failed channel" },
        failure: { code: "timed_out", message: "Channel generation took too long. Try again." },
      }),
      job({
        jobId: "submitted",
        status: "done",
        attempts: 1,
        intent: { description: "Submitted channel" },
        proposal: proposal({ id: "submitted-proposal", jobId: "submitted" }),
      }),
      job({
        jobId: "approved",
        status: "done",
        attempts: 1,
        intent: { description: "Approved channel" },
        proposal: proposal({ id: "approved-proposal", jobId: "approved", status: "approved" }),
      }),
      job({
        jobId: "denied",
        status: "done",
        attempts: 1,
        intent: { description: "Denied channel" },
        proposal: proposal({ id: "denied-proposal", jobId: "denied", status: "denied" }),
      }),
    ]);
    render(<MyRequests />);

    expect(await screen.findByText("Queued channel")).toBeInTheDocument();
    expect(screen.getByText("Running channel")).toBeInTheDocument();
    expect(screen.getByText("Failed channel")).toBeInTheDocument();
    expect(screen.getByText("Waiting for approval")).toBeInTheDocument();
    expect(screen.getByText("Approved")).toBeInTheDocument();
    expect(screen.getByText("Not approved")).toBeInTheDocument();
  });

  // The tracked-titles table below is the page's real content; an "you have asked for nothing"
  // panel above it would be noise on the common path.
  it("renders nothing when the member has no requests", async () => {
    stubProposalJobs([]);
    const { container } = render(<MyRequests />);

    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });
});
