import type { ProposalJourneyDTO, TitleDTO } from "@loomarr/api";
import { describe, expect, it } from "vitest";
import { requestFixLabel, requestStatus } from "./request-status";

const journey = (over: Partial<ProposalJourneyDTO> = {}): ProposalJourneyDTO => ({
  version: 1,
  jobId: "job-1",
  milestone: "live",
  intent: { description: "Saturday morning cartoons" },
  attempts: [],
  actions: [],
  createdAt: "2026-09-24T18:00:00Z",
  updatedAt: "2026-09-24T18:00:00Z",
  ...over,
});

const withAcquisitions = (milestone: ProposalJourneyDTO["milestone"], ids: number[]): ProposalJourneyDTO =>
  journey({
    milestone,
    proposal: {
      id: "p-1",
      status: "approved",
      proposal: {
        intent: { description: "x" },
        lineup: [],
        alternates: [],
        acquisitions: ids.map((tmdbId) => ({
          mediaType: "movie",
          name: `Movie ${tmdbId}`,
          tmdbId,
          inLibrary: false,
        })),
      } as never,
    },
  });

const title = (tmdbId: number, state: TitleDTO["state"]): TitleDTO => ({
  key: `movie:tmdb:${tmdbId}`,
  mediaType: "movie",
  tmdbId,
  state,
});

describe("requestFixLabel", () => {
  it("offers one action, preferring an edit over a plain retry", () => {
    expect(requestFixLabel(journey({ actions: ["retry", "edit"] }))).toBe("Edit and try again");
    expect(requestFixLabel(journey({ actions: ["retry"] }))).toBe("Try again");
    expect(requestFixLabel(journey({ actions: [] }))).toBeUndefined();
  });

  it("names the reference when that is what needs editing", () => {
    const failure = {
      code: "selection_empty",
      reason: "reference_unreadable",
      recoveryAction: "edit_reference",
      message: "m",
      guidance: "g",
    } as const;
    expect(requestFixLabel(journey({ actions: ["edit"], failure }))).toBe("Edit reference");
  });
});

describe("requestStatus", () => {
  it("files a failed request under Needs you with the server's reason", () => {
    const s = requestStatus(
      journey({
        milestone: "failed",
        failure: {
          code: "generation_failed",
          reason: "provider_timeout",
          recoveryAction: "retry_later",
          message: "The model took too long.",
          guidance: "Try again in a moment.",
        },
      }),
      [],
    );
    expect(s.tab).toBe("needs-you");
    expect(s.line).toBe("Couldn't build");
    expect(s.detail).toBe("The model took too long.");
  });

  // Deleting a channel is deliberate, so an approved request whose channel is gone is Done, with a
  // short badge; the sentence lives in `detail`.
  it("files an approved request whose channel is gone under Done as 'Channel removed'", () => {
    const s = requestStatus(
      {
        ...withAcquisitions("failed", [1]),
        failure: {
          code: "generation_failed",
          reason: "generation_failed",
          recoveryAction: "retry_later",
          message: "This request was approved, but its channel no longer exists.",
          guidance: "Try again to build a new channel from this request.",
        },
      },
      [],
    );
    expect(s).toMatchObject({
      tab: "done",
      line: "Channel removed",
      detail: "This request was approved, but its channel no longer exists.",
    });
  });

  it("keeps a request that is generating or awaiting approval In progress", () => {
    expect(requestStatus(journey({ milestone: "generating" }), [])).toMatchObject({
      tab: "in-progress",
      line: "Generating",
    });
    expect(requestStatus(journey({ milestone: "awaiting_approval" }), [])).toMatchObject({
      tab: "in-progress",
      line: "Waiting for approval",
    });
  });

  it("reports how many titles are still coming and in which state", () => {
    const s = requestStatus(withAcquisitions("live", [1, 2, 3, 4]), [
      title(1, "downloading"),
      title(2, "requested"),
      title(3, "wanted"),
      title(4, "available"),
    ]);
    expect(s.tab).toBe("in-progress");
    expect(s.line).toBe("Getting 3 titles (2 downloading, 1 waiting)");
  });

  it("says Couldn't get N once nothing is still on its way", () => {
    const s = requestStatus(withAcquisitions("live", [1, 2]), [
      title(1, "unavailable"),
      title(2, "available"),
    ]);
    expect(s).toMatchObject({ tab: "in-progress", line: "Couldn't get 1 title" });
  });

  it("is Done once the channel is live and every title has landed — no separate 'Ready' state", () => {
    const s = requestStatus(withAcquisitions("live", [1]), [title(1, "available")]);
    expect(s).toMatchObject({ tab: "done", line: "On your channel" });
  });

  it("files a declined request under Done", () => {
    expect(requestStatus(journey({ milestone: "denied" }), [])).toMatchObject({
      tab: "done",
      line: "Not approved",
    });
  });
});
