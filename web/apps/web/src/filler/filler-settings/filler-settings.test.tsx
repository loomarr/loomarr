import {
  getFillerReadinessMockHandler,
  getListFillerSourcesMockHandler,
  getSettingsListMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { SettingsEditsProvider } from "@/settings/settings-edits";
import { setting } from "@/test/fixtures/settings";
import { server } from "@/test/msw/server";
import { RouterHarness } from "@/test/story-utils";
import type { FillerSettingsSection } from "../filler-settings-section";
import { FillerSettings, FillerSettingsIndex } from "./filler-settings";

const renderSettings = (section: FillerSettingsSection = "downloads") => {
  server.use(
    getListFillerSourcesMockHandler({ sources: [], total: 0 }),
    getFillerReadinessMockHandler({
      ready: true,
      nextAction: "none",
      repairs: { count: 0 },
      fetch: { enabled: true, catalogClips: 12 },
      storage: {
        automatic: true,
        state: "healthy",
        totalBytes: 500 * 1024 ** 3,
        freeBytes: 200 * 1024 ** 3,
        managedBytes: 2 * 1024 ** 3,
        reservedBytes: 0,
        filesystemReservedBytes: 0,
        softBudgetBytes: 20 * 1024 ** 3,
        hardReserveBytes: 10 * 1024 ** 3,
        availableBytes: 18 * 1024 ** 3,
      },
      pipeline: {
        runnable: 0,
        scheduled: 0,
        inProgress: 0,
        needsDecision: 0,
        recoverable: 0,
        ready: 12,
        complete: 0,
        rejected: 0,
        dismissed: 0,
      },
      pool: { clips: 12, breakBody: 10, eligible: 10, untagged: 0, channels: [] },
      acquisitions: [],
    }),
    getSettingsListMockHandler({
      features: { filler: true },
      settings: [
        setting({
          key: "filler.fetch.every",
          label: "Check frequency",
          value: "6h",
          kind: "duration",
          group: "filler",
        }),
        setting({
          key: "filler.fetch.max_per_run",
          label: "New clips",
          value: "10",
          kind: "int",
          group: "filler",
        }),
        setting({
          key: "filler.fetch.max_catalog_clips",
          label: "Catalog limit",
          value: "500",
          kind: "int",
          group: "filler",
          advanced: true,
        }),
        setting({
          key: "filler.storage.library_budget_gb",
          label: "Filler storage allowance",
          value: "0",
          kind: "int",
          group: "filler",
          doc: "How much space Loomarr may use for filler. Leave at 0 to choose automatically.",
        }),
        setting({
          key: "filler.incoming.ready_window",
          label: "Keep ready clips in Incoming",
          value: "24h",
          kind: "duration",
          group: "filler",
          advanced: true,
        }),
      ],
    }),
  );
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <SettingsEditsProvider>
        <RouterHarness initialPath="/filler" content={<FillerSettings section={section} />} />
      </SettingsEditsProvider>
    </QueryClientProvider>,
  );
};

describe("focused Filler settings", () => {
  it("defaults to downloads without showing unrelated storage fields", async () => {
    renderSettings();
    expect(await screen.findByRole("spinbutton", { name: "New clips" })).toHaveValue(10);
    expect(screen.getByRole("heading", { name: "Automatic downloads" })).toBeInTheDocument();
    expect(screen.queryByRole("spinbutton", { name: "Storage limit" })).not.toBeInTheDocument();
    expect(
      screen.getByText(/No enabled sources are currently downloading automatically/),
    ).toBeInTheDocument();
  });

  it("shows automatic storage simply and keeps the catalog ceiling advanced", async () => {
    renderSettings("storage");
    expect(await screen.findByRole("spinbutton", { name: "Filler storage allowance" })).toHaveValue(0);
    expect(screen.getByText("Automatic 20.0 GB allowance")).toBeInTheDocument();
    expect(screen.getByText(/2.0 GB used · 18.0 GB available to Loomarr/)).toBeInTheDocument();
    expect(screen.queryByRole("spinbutton", { name: "Catalog limit" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show advanced (1)" })).toBeInTheDocument();
    expect(screen.queryByRole("spinbutton", { name: "New clips" })).not.toBeInTheDocument();
  });

  it("opens the Incoming history task with one human duration control", async () => {
    renderSettings("incoming");
    expect(await screen.findByRole("heading", { name: "Incoming history" })).toBeInTheDocument();
    expect(screen.getByRole("spinbutton", { name: "Keep ready clips in Incoming" })).toHaveValue(1);
    expect(screen.getByRole("combobox", { name: "Keep ready clips in Incoming unit" })).toHaveTextContent(
      "days",
    );
    expect(screen.queryByRole("spinbutton", { name: "New clips" })).not.toBeInTheDocument();
  });

  it("switches tasks near the heading instead of burying them at the bottom", async () => {
    renderSettings("storage");
    expect(await screen.findByRole("combobox", { name: "Filler settings task" })).toHaveTextContent(
      "Storage limits",
    );
    expect(screen.getByRole("link", { name: "All filler settings" })).toHaveAttribute(
      "href",
      "/filler/settings",
    );
    expect(screen.queryByRole("button", { name: /more settings/i })).not.toBeInTheDocument();
  });
});

describe("Filler settings index", () => {
  it("shows every task in an everyday or advanced group", async () => {
    render(<RouterHarness initialPath="/filler/settings" content={<FillerSettingsIndex />} />);

    expect(await screen.findByRole("heading", { name: "Everyday choices" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Advanced tuning" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Automatic downloads/ })).toHaveAttribute(
      "href",
      "/filler/settings/downloads",
    );
    expect(screen.getByRole("link", { name: /Processing tools/ })).toHaveAttribute(
      "href",
      "/filler/settings/tools",
    );
    expect(screen.getByRole("link", { name: "Manage" })).toHaveAttribute("href", "/filler/manage");
  });
});
