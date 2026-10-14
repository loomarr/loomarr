import type { Intent, ProposalItem, ProposalJourneyDTO, Scores } from "@loomarr/api";
import { getGetProposalJobMockHandler, getReviseProposalJobMockHandler } from "@loomarr/api/msw";
import type { EventHandlers } from "@loomarr/core/events";
import { provisionKey } from "@loomarr/core/provision";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { isSuggestionExpansion } from "../suggestion-expansion";
import { useProposalReviewEdit } from "../use-proposal-review-edit";
import { useSuggestionRun } from "../use-suggestion-run";
import { useAiResume } from "./use-ai-resume";

// The real provider is an EventSource jsdom cannot run; the Journey poll is what moves this test.
let handlers: EventHandlers | undefined;
vi.mock("@/events/events-provider", () => ({
  useLoomarrEventListener: (h: EventHandlers) => {
    handlers = h;
  },
}));

const makeWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const movie = (name: string, year: number, tmdbId: number, inLibrary = true): ProposalItem => ({
  name,
  year,
  tmdbId,
  mediaType: "movie",
  inLibrary,
});

// Invented titles: two picked by hand, two the model offers afterwards.
const picks = [movie("Harbor Lights", 1994, 910001), movie("Copper Canyon Run", 1997, 910002, false)];
const offered = [movie("Lantern Street", 1995, 910003), movie("The Quiet Ferry", 1998, 910004)];

const scores: Scores = {
  version: 1,
  themeFit: 1,
  availabilityRatio: 0.5,
  eraBalance: 1,
  theme: { status: "supported", basis: "qualifiers", assessedItems: 2, unknownItems: 0, qualifiers: [] },
  era: { status: "supported", assessedItems: 2, matchingItems: 2, unknownItems: 0 },
};

// The manual builder's submit has already recorded the picks: a submitted, hand-built proposal
// whose intent is the channel name, as POST /v1/discovery/manual-channel records it.
const manualJourney = (): ProposalJourneyDTO => ({
  version: 1,
  jobId: "job-manual",
  milestone: "awaiting_approval",
  intent: { description: "Rainy Night Mysteries" },
  attempts: [],
  proposal: {
    id: "proposal-manual",
    status: "submitted",
    proposal: {
      intent: { description: "Rainy Night Mysteries" },
      channelName: "Rainy Night Mysteries",
      manual: true,
      lineup: [picks[0] as ProposalItem],
      acquisitions: [picks[1] as ProposalItem],
      alternates: [],
      scores,
      trace: { version: 1, surfacedTotal: 0, recordedTotal: 0, truncated: false, candidates: [] },
    },
  },
  actions: ["review", "edit"],
  createdAt: "2026-10-13T12:00:00Z",
  updatedAt: "2026-10-13T12:00:00Z",
});

const useResumeHarness = () => {
  const run = useSuggestionRun();
  const [edit, , optionalSuggestions] = useProposalReviewEdit(run.jobId, run.proposal);
  const resume = useAiResume(run, edit, optionalSuggestions);
  return { run, edit, optionalSuggestions, ...resume };
};

describe("useAiResume", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
    handlers = undefined;
  });

  // Resumes the manual Job with AI, captures the one expansion revision, then lands the model's
  // answer: new titles only, as the expansion prompt asks, named as the server names it (the
  // revision's fixed name when it carries one, else the model's own).
  const resumeAndExpand = async (channelName?: string) => {
    const journey = manualJourney();
    const revisions: Intent[] = [];
    server.use(
      getGetProposalJobMockHandler(() => journey),
      getReviseProposalJobMockHandler(async ({ request }) => {
        const revised = (await request.json()) as Intent;
        revisions.push(revised);
        journey.intent = revised;
        journey.milestone = "generating";
        journey.actions = ["wait"];
        return { jobId: "job-manual" };
      }),
    );
    const hook = renderHook(useResumeHarness, { wrapper: makeWrapper() });

    act(() =>
      hook.result.current.resumeWithAi("job-manual", "Moody mysteries for a rainy evening", channelName),
    );

    await waitFor(() => expect(revisions).toHaveLength(1));
    const [revision] = revisions;

    journey.milestone = "awaiting_approval";
    journey.actions = ["review", "edit"];
    journey.proposal = {
      id: "proposal-expanded",
      status: "submitted",
      proposal: {
        intent: revision as Intent,
        channelName: revision?.channelName || "Rainy Village Mysteries",
        lineup: offered,
        acquisitions: [],
        alternates: [],
        scores,
        trace: { version: 1, surfacedTotal: 2, recordedTotal: 2, truncated: false, candidates: [] },
      },
    };
    await act(async () => {
      handlers?.onSuggestion?.({ jobId: "job-manual", phase: "done", round: 0 });
    });
    await waitFor(() => expect(hook.result.current.run.proposal?.id).toBe("proposal-expanded"));
    return { ...hook, revision, revisions };
  };

  it("asks AI for more titles and keeps every hand-picked title in the lineup", async () => {
    const { result, revision, revisions } = await resumeAndExpand("Rainy Night Mysteries");

    // One expansion revision, fired only after the manual review landed: the brief the person
    // wrote, the picks as the kept lineup, and the dedicated option-expansion refinement.
    expect(revision?.description).toBe("Moody mysteries for a rainy evening");
    expect(revision?.currentLineup?.map((item) => item.key)).toEqual(picks.map(provisionKey));
    expect(isSuggestionExpansion(revision?.refineText)).toBe(true);

    // No pick lost: both stay selected (carried as additions over the replacement), and the AI
    // titles arrive as optional suggestions rather than replacing them.
    await waitFor(() => expect(result.current.edit?.add?.map(provisionKey)).toEqual(picks.map(provisionKey)));
    expect(result.current.edit?.drop?.sort()).toEqual(offered.map(provisionKey).sort());
    expect(result.current.optionalSuggestions.map(provisionKey)).toEqual(offered.map(provisionKey));
    expect(result.current.resuming).toBe(false);
    expect(revisions).toHaveLength(1);
  });

  it("keeps the typed channel name through the expansion", async () => {
    const { result, revision } = await resumeAndExpand("  Rainy Night Mysteries ");

    // The typed name is a fixed field of the revision, so the server keeps it over the model's.
    expect(revision?.channelName).toBe("Rainy Night Mysteries");
    expect(result.current.run.proposal?.proposal.channelName).toBe("Rainy Night Mysteries");
  });

  it("lets the AI name the channel when no name was typed", async () => {
    const { result, revision } = await resumeAndExpand();

    expect(revision).not.toHaveProperty("channelName");
    expect(result.current.run.proposal?.proposal.channelName).toBe("Rainy Village Mysteries");
  });
});
