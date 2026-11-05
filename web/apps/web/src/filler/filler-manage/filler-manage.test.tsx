import {
  getFillerDefectReportMockHandler,
  getFillerReadinessMockHandler,
  getFillerWatchMockHandler,
  getMeMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it } from "vitest";
import { defectReport, readiness } from "@/test/fixtures/filler";
import { me } from "@/test/fixtures/users";
import { server } from "@/test/msw/server";
import { RouterHarness } from "@/test/story-utils";
import { FillerManage } from "./filler-manage";

const wrapper = ({ children }: { children: ReactNode }) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={client}>
      <RouterHarness content={children} initialPath="/filler/manage" />
    </QueryClientProvider>
  );
};

describe("FillerManage", () => {
  // The hub reads the watch line and readiness on every visit. A quiet install by default; tests
  // about the hub override these.
  beforeEach(() => {
    server.use(
      getFillerWatchMockHandler({
        clips: 25,
        health: "healthy",
        held: 0,
        sourcesOn: 1,
        sourcesReady: 1,
        sourcesTotal: 1,
      }),
      getFillerReadinessMockHandler(readiness()),
      getFillerDefectReportMockHandler(defectReport()),
    );
  });

  it("opens the complete settings index instead of exposing one buried task", async () => {
    server.use(getMeMockHandler(me({ name: "Admin" })));
    render(<FillerManage />, { wrapper });

    expect(await screen.findByRole("link", { name: "Open Settings" })).toHaveAttribute(
      "href",
      "/filler/settings",
    );
    expect(screen.queryByRole("link", { name: "Automatic download settings" })).not.toBeInTheDocument();
  });

  // The web mock's Manage hub: one row per tool, each summary read from what the server serves.
  it("lists the admin's tools with their live state", async () => {
    server.use(
      getMeMockHandler(me({ name: "Admin" })),
      // `held` is what the header's "N need you" counts; Incoming's row must say the same number.
      getFillerWatchMockHandler({
        clips: 40,
        health: "healthy",
        held: 3,
        sourcesOn: 1,
        sourcesReady: 1,
        sourcesTotal: 2,
      }),
      getFillerReadinessMockHandler(readiness({ pool: { ...readiness().pool, untagged: 5 } })),
      getFillerDefectReportMockHandler(
        defectReport({ verdicts: { pass: 30, fail: 7, undecided: 0, pending: 3 }, total: 12 }),
      ),
    );
    render(<FillerManage />, { wrapper });

    const tools = await screen.findByRole("region", { name: "Filler tools" });
    expect(await within(tools).findByText("1 of 2 on")).toBeInTheDocument();
    expect(await within(tools).findByText("3 clips need a choice")).toBeInTheDocument();
    expect(await within(tools).findByText("7 would be retired · 12 clips to label")).toBeInTheDocument();
    expect(within(tools).getByRole("link", { name: "Open Quality" })).toHaveAttribute(
      "href",
      "/filler/quality",
    );
    expect(
      within(tools).getByText("Product, format, season and audience · 5 clips not tagged yet"),
    ).toBeInTheDocument();
    expect(within(tools).getByRole("link", { name: "Open Sources" })).toHaveAttribute(
      "href",
      "/filler/sources",
    );
    expect(within(tools).getByRole("link", { name: "Open Incoming" })).toHaveAttribute(
      "href",
      "/filler/incoming",
    );
    expect(within(tools).getByRole("link", { name: "Open Tags" })).toHaveAttribute(
      "href",
      "/filler/taxonomy",
    );
  });

  it("shows a member only the tools they can use", async () => {
    server.use(getMeMockHandler(me({ name: "Viewer", role: "member" })));
    render(<FillerManage />, { wrapper });

    const tools = await screen.findByRole("region", { name: "Filler tools" });
    expect(await within(tools).findByText("Tags")).toBeInTheDocument();
    expect(within(tools).getByText("Settings")).toBeInTheDocument();
    expect(within(tools).queryByText("Sources")).not.toBeInTheDocument();
    expect(within(tools).queryByText("Quality")).not.toBeInTheDocument();
    expect(within(tools).queryByText("Incoming")).not.toBeInTheDocument();
    expect(within(tools).queryByRole("link", { name: "Open Settings" })).not.toBeInTheDocument();
  });
});
