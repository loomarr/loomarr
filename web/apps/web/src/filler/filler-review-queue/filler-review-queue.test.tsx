import { getActOnFillerDecisionMockHandler, getFillerDecisionReviewsMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { RouterHarness } from "@/test/story-utils";
import { FillerReviewQueue } from "./filler-review-queue";

const review = {
  id: "decision-1",
  clipHash: "abcdef0123456789",
  createdAt: "2026-08-25T12:00:00Z",
  question: "Is this a soda commercial?",
  reasonCodes: ["brand_category_conflict"],
  evidenceRefs: ["transcript", "frame-2"],
  conflicts: [
    {
      claim: "product category",
      values: ["Mountain Dew", "unknown"],
      evidenceRefs: ["transcript", "frame-2"],
      resolved: false,
    },
  ],
};

const wrapper = ({ children }: { children: ReactNode }) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <RouterHarness content={children} initialPath="/filler/attention" />
    </QueryClientProvider>
  );
};

describe("FillerReviewQueue", () => {
  it("renders one plain evidence-first question without operational state", async () => {
    server.use(getFillerDecisionReviewsMockHandler({ rows: [review], total: 1 }));
    render(<FillerReviewQueue />, { wrapper });

    expect(await screen.findByRole("heading", { name: "Is this a soda commercial?" })).toBeInTheDocument();
    expect(screen.getByText("Mountain Dew · unknown")).toBeInTheDocument();
    expect(screen.getByText("2 evidence sources")).toBeInTheDocument();
    expect(screen.queryByText(/provider|budget|retry/i)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Accept" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Correct" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reject" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Skip for now" })).toBeInTheDocument();
  });

  it("records skip for now without inventing an accept or reject answer", async () => {
    const bodies: unknown[] = [];
    server.use(
      getFillerDecisionReviewsMockHandler({ rows: [review], total: 1 }),
      getActOnFillerDecisionMockHandler(async ({ request }) => {
        bodies.push(await request.json());
        return { id: "action-skip" };
      }),
    );
    render(<FillerReviewQueue />, { wrapper });

    await userEvent.click(await screen.findByRole("button", { name: "Skip for now" }));

    await waitFor(() =>
      expect(bodies).toEqual([{ actionId: expect.any(String), kind: "abandon", reason: "skip for now" }]),
    );
    expect(await screen.findByText("You're caught up for now")).toBeInTheDocument();
    expect(screen.getByText(/did not treat them as accepted or rejected/i)).toBeInTheDocument();
  });

  it("records a correction as a distinct append-only action", async () => {
    const bodies: unknown[] = [];
    server.use(
      getFillerDecisionReviewsMockHandler({ rows: [review], total: 1 }),
      getActOnFillerDecisionMockHandler(async ({ request }) => {
        bodies.push(await request.json());
        return { id: "action-1" };
      }),
    );
    render(<FillerReviewQueue />, { wrapper });

    await userEvent.click(await screen.findByRole("button", { name: "Correct" }));
    expect(screen.getByLabelText("Correction")).toHaveFocus();
    await userEvent.click(screen.getByLabelText("It is filler"));
    await userEvent.type(screen.getByLabelText("Correction"), "This is a soda commercial");
    await userEvent.click(screen.getByRole("button", { name: "Save correction" }));

    await waitFor(() => expect(bodies).toHaveLength(1));
    expect(bodies[0]).toMatchObject({
      kind: "correct",
      correctedVerdict: "admit",
      answer: "This is a soda commercial",
    });
  });

  it("records admission without inventing an answer to an open question", async () => {
    const bodies: Record<string, unknown>[] = [];
    server.use(
      getFillerDecisionReviewsMockHandler({
        rows: [{ ...review, question: "Which product is this commercial advertising?" }],
        total: 1,
      }),
      getActOnFillerDecisionMockHandler(async ({ request }) => {
        bodies.push((await request.json()) as Record<string, unknown>);
        return { id: "action-admit" };
      }),
    );
    render(<FillerReviewQueue />, { wrapper });

    await userEvent.click(await screen.findByRole("button", { name: "Accept" }));

    await waitFor(() => expect(bodies).toHaveLength(1));
    expect(bodies[0]).toMatchObject({ actionId: expect.any(String), kind: "admit" });
    expect(bodies[0]).not.toHaveProperty("answer");
  });

  it("renders a healthy zero-work state", async () => {
    server.use(getFillerDecisionReviewsMockHandler({ rows: [], total: 0 }));
    render(<FillerReviewQueue />, { wrapper });
    expect(await screen.findByText("Nothing needs your attention")).toBeInTheDocument();
  });
});
