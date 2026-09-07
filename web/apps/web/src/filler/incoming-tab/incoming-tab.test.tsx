import type { FillerIncomingOutputBody, IncomingClipDTO } from "@loomarr/api";
import { getFillerIncomingMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { IncomingTab } from "./incoming-tab";

// IncomingTab owns the review queue and its safe lifecycle mutations. It was extracted from
// `filler-page.tsx`, where all of it mounted on every tab — so a member reading the Catalog
// still paid for a queue that is admin-only server-side.
//
// ⚠ Typed as IncomingClipDTO, which surfaced two MISSING required fields (`kind`, `reason`) the
// untyped stub happily accepted.
//
// ⚠ `needsDecision` is what puts this clip on the OPERATOR's end of the belt (§10 V51e). Without
// it the row renders as still-being-prepared and carries no controls at all — so every assertion
// below about buttons and PATCH bodies would fail for a reason that looks nothing like the cause.
const ASK: IncomingClipDTO = {
  kind: "commercial",
  reason: "era-guess",
  needsDecision: true,
  path: "a3/f9/abc.mp4",
  hash: "hash-abc",
  name: "Toy ad",
  suggestedEra: 1993,
  audience: "kids",
  category: "toys",
  confidence: 80,
  durationMs: 30_000,
};

// ⚠ The stub this replaced ended in `return Promise.resolve(jsonResponse(200, {}))` — a catch-all
// answering any other url with an empty object — and its assertions then searched
// `calls.find((c) => c.method === "PATCH")`. Both are the weakness this migration removes: the
// catch-all could not fail, and a method filter would match a PATCH to ANY endpoint. Since the
// whole point of this file is that the PATCH carries the right BODY to the right route, binding
// the handler to `*/v1/filler/tags` is the assertion the old test could not make.
const stubIncoming = (incoming: Partial<FillerIncomingOutputBody> = {}) => {
  const body: FillerIncomingOutputBody = {
    clips: [ASK],
    reels: [],
    // ⚠ `rejected` and `stageOrder` are REQUIRED and the old stub supplied NEITHER — two more
    // fields an untyped catch-all let through.
    rejected: [],
    stageOrder: [],
    total: 1,
    ...incoming,
    clipsTotal: incoming.clipsTotal ?? incoming.clips?.length ?? 1,
    decisionsTotal:
      incoming.decisionsTotal ?? incoming.clips?.filter((clip) => clip.needsDecision).length ?? 1,
    reelsTotal: incoming.reelsTotal ?? incoming.reels?.length ?? 0,
    rejectedTotal: incoming.rejectedTotal ?? incoming.rejected?.length ?? 0,
    overview:
      incoming.overview ??
      ({
        runnable: 0,
        inProgress: 0,
        scheduled: 0,
        needsDecision: 1,
        recoverable: 0,
        admitted: 0,
        rejected: 0,
        dismissed: 0,
      } as const),
  };
  server.use(getFillerIncomingMockHandler(body));
};

const makeWrapper = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const renderTab = (onEditTags = vi.fn()) => {
  render(<IncomingTab onEditTags={onEditTags} />, { wrapper: makeWrapper() });
  return { onEditTags };
};

describe("IncomingTab", () => {
  it("fetches the incoming queue itself rather than being handed it", async () => {
    stubIncoming();
    renderTab();
    // The whole point of the extraction: the tab is self-sufficient, so the shell no longer
    // unpacks a queue for a tab that may not be showing. Rendering the ask IS the proof the
    // fetch happened — and an unmatched request would now fail the test by name, where the old
    // catch-all would have answered it silently.
    expect(await screen.findByText("Toy ad")).toBeInTheDocument();
    expect(screen.getByRole("region", { name: /filler pipeline status/i })).toHaveTextContent(
      /1 clip decisions/i,
    );
  });

  it("does not expose a classification shortcut as a publication decision", async () => {
    stubIncoming();
    renderTab();
    await screen.findByText("Toy ad");

    expect(screen.queryByRole("button", { name: /looks right/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^use it$/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /file all/i })).not.toBeInTheDocument();
  });

  it("hands tag editing up to the shell, which owns the one dialog", async () => {
    stubIncoming();
    const { onEditTags } = renderTab();
    await screen.findByText("Toy ad");

    await userEvent.click(await screen.findByRole("button", { name: /review tags/i }));

    // ⚠ The HASH (§10 V54). This asserted `ASK.path` and was green while the button did nothing:
    // the shell resolves the clip by identity, so a path matched no row and no dialog opened. The
    // fixture's path and hash are deliberately different strings — equate them and this test
    // cannot tell the two apart, which is the same trap `putClip` sets on the Go side.
    expect(onEditTags).toHaveBeenCalledWith(ASK.hash);
  });

  it("renders an empty queue without erroring", async () => {
    stubIncoming({ clips: [], total: 0 });
    renderTab();
    await waitFor(() => expect(screen.queryByText("Toy ad")).not.toBeInTheDocument());
  });

  it("does not render a clip twice when the semantic review card owns it", async () => {
    stubIncoming();
    render(
      <IncomingTab onEditTags={vi.fn()} excludedHashes={new Set([ASK.hash])} semanticReviewCount={1} />,
      { wrapper: makeWrapper() },
    );

    expect(await screen.findByLabelText(/filler pipeline status/i)).toHaveTextContent(/1 clip decisions/i);
    expect(screen.queryByText("Toy ad")).not.toBeInTheDocument();
    expect(screen.queryByText("Nothing needs you")).not.toBeInTheDocument();
  });
});
