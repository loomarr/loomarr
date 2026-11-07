import type { ProposalJourneyDTO } from "@loomarr/api";
import { getGetProposalJobMockHandler, getSubmitProposalMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useLocation, useSearch } from "@tanstack/react-router";
import { act, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { RouterHarness } from "@/test/story-utils";
import { parseOriginationSearch } from "../origination-flow";
import { readActiveSuggestionJob, readSuggestionDraft, writeSuggestionDraft } from "../suggestion-draft";
import { type OriginationFlow, useOriginationFlow } from "./use-origination-flow";

const journey: ProposalJourneyDTO = {
  version: 1,
  jobId: "job-1",
  milestone: "generating",
  intent: { description: "90s action movies" },
  attempts: [{ version: 1, number: 1, status: "running", startedAt: "2026-08-22T12:00:00Z" }],
  actions: ["wait"],
  createdAt: "2026-08-22T12:00:00Z",
  updatedAt: "2026-08-22T12:00:00Z",
};

// Renders the flow over a real router and hands back the latest flow and URL.
const renderFlow = (initialPath: string) => {
  const latest: { flow?: OriginationFlow; href?: string } = {};
  const Probe = () => {
    latest.flow = useOriginationFlow(parseOriginationSearch(useSearch({ strict: false })));
    latest.href = useLocation({ select: (location) => location.href });
    return <p>{latest.flow.open ? "open" : "closed"}</p>;
  };
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <RouterHarness initialPath={initialPath} content={<Probe />} />
    </QueryClientProvider>,
  );
  return latest;
};

const step = async (action: (flow: OriginationFlow) => void, latest: { flow?: OriginationFlow }) => {
  await waitFor(() => expect(latest.flow).toBeDefined());
  act(() => action(latest.flow as OriginationFlow));
};

describe("useOriginationFlow", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
    server.use(getSubmitProposalMockHandler({ jobId: "job-1" }), getGetProposalJobMockHandler(journey));
  });

  it("moves the URL onto the Job a submit creates, and remembers it for the tab", async () => {
    const latest = renderFlow("/guide?new=1&intent=cartoons");

    await step((flow) => flow.run.start({ description: "90s action movies" }), latest);

    await waitFor(() => expect(latest.href).toBe("/guide?job=job-1"));
    expect(readActiveSuggestionJob()).toBe("job-1");
  });

  // Close means "come back later": the Job survives it, so reopening lands on the same run.
  it("closes without forgetting the Job, and reopens onto it", async () => {
    const latest = renderFlow("/guide?job=job-1");
    await waitFor(() => expect(readActiveSuggestionJob()).toBe("job-1"));

    await step((flow) => flow.close(), latest);
    await waitFor(() => expect(screen.getByText("closed")).toBeInTheDocument());
    expect(latest.href).toBe("/guide");

    await step((flow) => flow.openPanel(), latest);
    await waitFor(() => expect(latest.href).toBe("/guide?job=job-1"));
  });

  it("drops an unsubmitted draft on close", async () => {
    writeSuggestionDraft({ description: "cartoons" });
    const latest = renderFlow("/guide?new=1");

    await step((flow) => flow.close(), latest);

    await waitFor(() => expect(latest.href).toBe("/guide"));
    expect(readSuggestionDraft()).toBeUndefined();
  });

  it("forgets the Job on a fresh start", async () => {
    const latest = renderFlow("/guide?job=job-1&intent=stale");
    await waitFor(() => expect(readActiveSuggestionJob()).toBe("job-1"));

    await step((flow) => flow.startFresh(), latest);

    await waitFor(() => expect(latest.flow?.search).toEqual({ new: "1" }));
    expect(readActiveSuggestionJob()).toBeUndefined();
  });

  it("returns from the manual builder to the failed Job it was entered from", async () => {
    const latest = renderFlow("/guide?job=job-1&manual=1");

    await step((flow) => flow.leaveManual(), latest);

    await waitFor(() => expect(latest.href).toBe("/guide?job=job-1"));
  });

  it("returns from a manual-only entry to the describe form", async () => {
    const latest = renderFlow("/guide?manual=1&ideaId=genre%3Acomedy");

    await step((flow) => flow.leaveManual(), latest);

    await waitFor(() => expect(latest.flow?.search).toEqual({ new: "1" }));
  });

  it("hands the builder's description to the describe form", async () => {
    const latest = renderFlow("/guide?manual=1");

    await step((flow) => flow.describeWithAi("Rainy night mysteries"), latest);

    await waitFor(() => expect(latest.flow?.search).toEqual({ new: "1", intent: "Rainy night mysteries" }));
  });
});
