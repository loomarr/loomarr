import type { FillerIncomingOutputBody, IncomingStatusDTO } from "@loomarr/api";
import { getFillerIncomingMockHandler, getListFillerMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { RouterHarness } from "@/test/story-utils";
import { Incoming } from "./incoming";

const status = (over: Partial<IncomingStatusDTO> = {}): IncomingStatusDTO => ({
  clipHash: "clip-1",
  name: "Tootsie Pop classic commercial",
  durationMs: 30_000,
  statusLabel: "Adding details",
  updatedAt: "2026-09-14T20:00:00Z",
  processing: { attempts: 1, stages: [] },
  ...over,
});

const incoming = (over: Partial<FillerIncomingOutputBody> = {}): FillerIncomingOutputBody =>
  ({
    preparing: { rows: [], total: 0 },
    needsHelp: { rows: [], total: 0 },
    recentlyReady: { rows: [], total: 0 },
    readyWindowSeconds: 86400,
    ...over,
  }) as FillerIncomingOutputBody;

const wrapper = ({ children }: { children: ReactNode }) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={client}>
      <RouterHarness content={children} initialPath="/filler/incoming" />
    </QueryClientProvider>
  );
};

describe("Incoming", () => {
  it("preserves simultaneous repeated processing steps without identity warnings", async () => {
    const error = vi.spyOn(console, "error");
    const stage = {
      label: "Adding details",
      outcome: "not_needed" as const,
      outcomeLabel: "Not needed",
      at: "2026-09-14T20:00:00Z",
      note: "This step was not needed for this clip.",
    };
    server.use(
      getFillerIncomingMockHandler(
        incoming({
          recentlyReady: {
            rows: [
              status({
                statusLabel: "Ready",
                processing: {
                  stages: [stage, { ...stage }, { ...stage, note: "Optional enrichment is disabled." }],
                },
              }),
            ],
            total: 1,
          },
        }),
      ),
      getListFillerMockHandler({ clips: [], total: 0 }),
    );
    try {
      render(<Incoming />, { wrapper });
      expect(await screen.findByText("This clip is ready whenever a channel needs it.")).toBeInTheDocument();
      await userEvent.click(await screen.findByRole("button", { name: /view details for tootsie pop/i }));
      await userEvent.click(screen.getByText("Processing details"));
      await userEvent.click(screen.getByRole("button", { name: "Show 3 skipped steps" }));
      const rows = within(screen.getByRole("dialog")).getAllByRole("listitem");
      expect(rows).toHaveLength(3);
      expect(rows.map((row) => within(row).getByText("Adding details").textContent)).toEqual([
        "Adding details",
        "Adding details",
        "Adding details",
      ]);
      expect(error).not.toHaveBeenCalled();
    } finally {
      error.mockRestore();
    }
  });

  it("leads with automatic progress and keeps a real choice distinct", async () => {
    server.use(
      getFillerIncomingMockHandler(
        incoming({
          preparing: {
            rows: [
              status(),
              status({ clipHash: "clip-2", name: "HP Sauce advert", statusLabel: "Checking video" }),
            ],
            total: 2,
          },
          needsHelp: {
            rows: [
              {
                id: "split-1",
                kind: "split_boundary",
                clipHash: "reel-1",
                name: "Saturday morning commercial reel",
                question: "Where should this compilation be split?",
                actionLabel: "Review clips",
                actionHref: "/filler/splits/split-1",
                createdAt: "2026-09-14T19:00:00Z",
              },
              {
                id: "split-2",
                kind: "split_boundary",
                clipHash: "reel-2",
                name: "Holiday commercial reel",
                question: "Where should this compilation be split?",
                actionLabel: "Review clips",
                actionHref: "/filler/splits/split-2",
                createdAt: "2026-09-14T18:00:00Z",
              },
            ],
            total: 2,
          },
          recentlyReady: {
            rows: [status({ clipHash: "ready-1", name: "Magna-Doodle commercial", statusLabel: "Ready" })],
            total: 1,
          },
        }),
      ),
    );

    render(<Incoming />, { wrapper });

    expect(await screen.findByRole("heading", { name: "2 clips are getting ready" })).toBeInTheDocument();
    expect(screen.getByText("Loomarr is taking care of these in the background.")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Needs your help" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Review clips" })).toHaveAttribute(
      "href",
      "/filler/splits/split-1",
    );
    expect(screen.queryByText("Holiday commercial reel")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(screen.getByText("Holiday commercial reel")).toBeInTheDocument();
    expect(screen.queryByText("Saturday morning commercial reel")).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Ready" })).toBeInTheDocument();
    expect(
      screen.getByText(/Added in the last 24 hours\. These clips stay in your Library\./),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Change" })).toHaveAttribute("href", "/filler/settings/incoming");
    expect(screen.getByRole("link", { name: "Open Library" })).toHaveAttribute("href", "/filler/library");
    expect(screen.queryByText(/shadow|admission|evidence hash|reason code/i)).not.toBeInTheDocument();
  });

  it("explains the empty state with one direct next step", async () => {
    server.use(getFillerIncomingMockHandler(incoming()));
    render(<Incoming />, { wrapper });

    expect(await screen.findByRole("heading", { name: "Nothing on the way" })).toBeInTheDocument();
    expect(screen.getByText("New clips will appear here after you add a source.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Add a source" })).toHaveAttribute("href", "/filler/sources");
  });

  it("opens calm details first and keeps processing history collapsed", async () => {
    server.use(
      getFillerIncomingMockHandler(
        incoming({
          preparing: {
            rows: [
              status({
                processing: {
                  attempts: 2,
                  nextTryAt: "2026-09-14T20:05:00Z",
                  diagnosticsHref: "/filler/manage#diagnostics",
                  stages: [
                    {
                      label: "Checking video",
                      outcome: "finished",
                      outcomeLabel: "Finished",
                      note: "This step finished successfully.",
                      at: "2026-09-14T19:58:00Z",
                    },
                    {
                      label: "Adding details",
                      outcome: "retrying",
                      outcomeLabel: "Trying again",
                      note: "This step did not finish. Loomarr will try again automatically.",
                      at: "2026-09-14T20:00:00Z",
                    },
                  ],
                },
              }),
            ],
            total: 1,
          },
        }),
      ),
      getListFillerMockHandler({ clips: [], total: 0 }),
    );
    render(<Incoming />, { wrapper });

    await userEvent.click(await screen.findByRole("button", { name: /view details for tootsie pop/i }));
    expect(screen.getByRole("heading", { name: "Tootsie Pop classic commercial" })).toBeInTheDocument();
    expect(
      screen.queryByText("This step did not finish. Loomarr will try again automatically."),
    ).not.toBeVisible();
    await userEvent.click(screen.getByText("Processing details"));
    expect(screen.getByText("This step was tried 2 times.")).toBeVisible();
    expect(screen.getByText("This step did not finish. Loomarr will try again automatically.")).toBeVisible();
    expect(screen.getByRole("link", { name: "View diagnostics" })).toHaveAttribute(
      "href",
      "/filler/manage#diagnostics",
    );
  });

  it("adds the next bounded page without replacing visible rows", async () => {
    let cursor = "";
    server.use(
      http.get("*/v1/filler/incoming", ({ request }) => {
        cursor = new URL(request.url).searchParams.get("preparingCursor") ?? "";
        return HttpResponse.json(
          incoming({
            preparing: cursor
              ? { rows: [status({ clipHash: "clip-21", name: "Clip 21" })], total: 21 }
              : { rows: [status({ clipHash: "clip-1", name: "Clip 1" })], total: 21, nextCursor: "page-2" },
          }),
        );
      }),
    );
    render(<Incoming />, { wrapper });

    expect(await screen.findByText("Clip 1")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Show more clips being prepared" }));
    expect(await screen.findByText("Clip 21")).toBeInTheDocument();
    expect(screen.getByText("Clip 1")).toBeInTheDocument();
    expect(cursor).toBe("page-2");
  });
});
