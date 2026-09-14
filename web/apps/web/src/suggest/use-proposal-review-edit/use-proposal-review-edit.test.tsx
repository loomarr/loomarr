import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { useProposalReviewEdit } from "./use-proposal-review-edit";

describe("useProposalReviewEdit", () => {
  beforeEach(() => window.sessionStorage.clear());

  it("restores pending title choices for the same proposal job", () => {
    window.sessionStorage.setItem(
      "loomarr.proposalReviewEdit.job-1",
      JSON.stringify({ drop: ["movie:tmdb:949"], note: "Keep the lighter picks" }),
    );
    const { result } = renderHook(() => useProposalReviewEdit("job-1"));
    expect(result.current[0]).toEqual({
      drop: ["movie:tmdb:949"],
      note: "Keep the lighter picks",
    });
  });

  it("isolates jobs and removes an edit when the review is reset", async () => {
    const { result, rerender } = renderHook(({ jobId }) => useProposalReviewEdit(jobId), {
      initialProps: { jobId: "job-1" as string | undefined },
    });
    act(() => result.current[1]({ drop: ["movie:tmdb:949"] }));
    expect(window.sessionStorage.getItem("loomarr.proposalReviewEdit.job-1")).toContain("tmdb:949");

    rerender({ jobId: "job-2" });
    await waitFor(() => expect(result.current[0]).toBeUndefined());
    act(() => result.current[1]({ note: "Second request" }));
    expect(window.sessionStorage.getItem("loomarr.proposalReviewEdit.job-1")).toContain("tmdb:949");
    expect(window.sessionStorage.getItem("loomarr.proposalReviewEdit.job-2")).toContain("Second request");

    act(() => result.current[1](undefined));
    expect(window.sessionStorage.getItem("loomarr.proposalReviewEdit.job-2")).toBeNull();
  });

  it("fails closed on a corrupt saved edit", () => {
    window.sessionStorage.setItem("loomarr.proposalReviewEdit.job-1", "not-json");
    const { result } = renderHook(() => useProposalReviewEdit("job-1"));
    expect(result.current[0]).toBeUndefined();
    expect(window.sessionStorage.getItem("loomarr.proposalReviewEdit.job-1")).toBeNull();
  });
});
