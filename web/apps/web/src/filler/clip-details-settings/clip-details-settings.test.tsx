import {
  getFillerResearchStatusMockHandler,
  getFillerResearchTestMockHandler,
  getSettingsPatchMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { setting } from "@/test/fixtures/settings";
import { server } from "@/test/msw/server";
import { ClipDetailsSettings } from "./clip-details-settings";

const entries = [
  setting({
    key: "filler.research.monthly_limit",
    label: "Monthly web searches",
    value: "100",
    kind: "int",
    group: "filler",
    owner: "filler.details",
    advanced: true,
    doc: "The most general-web searches Loomarr may make each month.",
  }),
  setting({
    key: "filler.research.searxng_url",
    label: "SearXNG address",
    value: "",
    kind: "url",
    group: "filler",
    owner: "filler.details",
    set: false,
  }),
];

const renderPanel = (values: Record<string, string> = {}) =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ClipDetailsSettings
        entries={entries}
        liveValue={(key) => values[key] ?? (key === "filler.research.enabled" ? "true" : "")}
        setEdit={vi.fn()}
      />
    </QueryClientProvider>,
  );

describe("ClipDetailsSettings", () => {
  it("keeps the resting state simple and validates before saving Brave Search", async () => {
    const tested: unknown[] = [];
    const saved: unknown[] = [];
    server.use(
      getFillerResearchStatusMockHandler({
        structuredEnabled: true,
        provider: "none",
        configured: false,
        state: "unconfigured",
        month: "2026-09",
        requestCount: 0,
        requestLimit: 100,
      }),
      getFillerResearchTestMockHandler(async ({ request }) => {
        tested.push(await request.json());
        return {
          ok: true,
          message: "Web search is ready.",
          status: {
            structuredEnabled: true,
            provider: "brave",
            configured: true,
            state: "ready",
            month: "2026-09",
            requestCount: 1,
            requestLimit: 100,
          },
        };
      }),
      getSettingsPatchMockHandler(async ({ request }) => {
        const body = (await request.json()) as { edits: Record<string, string> };
        saved.push(body);
        return { results: Object.keys(body.edits).map((key) => ({ key, status: "saved" as const })) };
      }),
    );
    renderPanel();

    expect(await screen.findByText("Clip details are ready")).toBeInTheDocument();
    expect(screen.getByText(/Web search is used only when they cannot identify a clip/)).toBeInTheDocument();
    expect(screen.queryByText("Monthly web searches")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Add web search" }));
    const sheet = await screen.findByRole("dialog");
    expect(within(sheet).getByRole("heading", { name: "Add web search" })).toBeInTheDocument();
    expect(within(sheet).getByRole("radio", { name: /Brave Search/ })).toBeChecked();
    await userEvent.type(within(sheet).getByLabelText("Brave Search API key"), "secret-key");
    await userEvent.click(within(sheet).getByRole("button", { name: "Test and add" }));

    expect(tested).toEqual([{ provider: "brave", apiKey: "secret-key" }]);
    expect(saved).toEqual([
      {
        edits: {
          "filler.research.web_provider": "brave",
          "filler.research.brave_api_key": "secret-key",
          "filler.research.searxng_url": "",
        },
      },
    ]);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("shows usage and keeps provider controls under Advanced when configured", async () => {
    const tested: unknown[] = [];
    server.use(
      getFillerResearchStatusMockHandler({
        structuredEnabled: true,
        provider: "brave",
        configured: true,
        state: "ready",
        month: "2026-09",
        requestCount: 3,
        requestLimit: 100,
        lastSuccessAt: "2026-09-20T12:00:00Z",
      }),
      getFillerResearchTestMockHandler(async ({ request }) => {
        tested.push(await request.json());
        return {
          ok: true,
          message: "Web search is ready.",
          status: {
            structuredEnabled: true,
            provider: "brave",
            configured: true,
            state: "ready",
            month: "2026-09",
            requestCount: 4,
            requestLimit: 100,
          },
        };
      }),
    );
    renderPanel({ "filler.research.monthly_limit": "100" });

    expect(await screen.findByText("Web search ready")).toBeInTheDocument();
    expect(screen.getByText(/Brave Search · 3 of 100 searches this month/)).toBeInTheDocument();
    expect(
      screen.getByRole("spinbutton", { name: "Monthly web searches" }).closest("details"),
    ).not.toHaveAttribute("open");
    await userEvent.click(screen.getByText("Advanced"));
    expect(screen.getByRole("spinbutton", { name: "Monthly web searches" })).toHaveValue(100);
    await userEvent.click(screen.getByRole("button", { name: "Test connection" }));
    expect(tested).toEqual([{ provider: "brave" }]);
    expect(await screen.findByText("Web search is ready.")).toBeInTheDocument();
  });

  it("keeps failed validation in the setup sheet and does not save", async () => {
    const saved: unknown[] = [];
    server.use(
      getFillerResearchStatusMockHandler({
        structuredEnabled: true,
        provider: "none",
        configured: false,
        state: "unconfigured",
        month: "2026-09",
        requestCount: 0,
        requestLimit: 100,
      }),
      getFillerResearchTestMockHandler({
        ok: false,
        message: "That key was not accepted.",
        status: {
          structuredEnabled: true,
          provider: "none",
          configured: false,
          state: "unconfigured",
          month: "2026-09",
          requestCount: 1,
          requestLimit: 100,
        },
      }),
      getSettingsPatchMockHandler(async ({ request }) => {
        saved.push(await request.json());
        return { results: [] };
      }),
    );
    renderPanel();

    await userEvent.click(await screen.findByRole("button", { name: "Add web search" }));
    const sheet = screen.getByRole("dialog");
    await userEvent.type(within(sheet).getByLabelText("Brave Search API key"), "bad-key");
    await userEvent.click(within(sheet).getByRole("button", { name: "Test and add" }));

    expect(await within(sheet).findByText("That key was not accepted.")).toBeInTheDocument();
    expect(saved).toEqual([]);
  });

  it("removes the provider and both stored provider details", async () => {
    const saved: Array<{ edits?: Record<string, string> }> = [];
    server.use(
      getFillerResearchStatusMockHandler({
        structuredEnabled: true,
        provider: "searxng",
        configured: true,
        state: "degraded",
        month: "2026-09",
        requestCount: 4,
        requestLimit: 100,
        lastFailureAt: "2026-09-20T12:00:00Z",
      }),
      getSettingsPatchMockHandler(async ({ request }) => {
        const body = (await request.json()) as { edits: Record<string, string> };
        saved.push(body);
        return { results: Object.keys(body.edits).map((key) => ({ key, status: "saved" as const })) };
      }),
    );
    renderPanel({ "filler.research.monthly_limit": "100" });

    expect(await screen.findByText("Web search needs a check")).toBeInTheDocument();
    await userEvent.click(screen.getByText("Advanced"));
    await userEvent.click(screen.getByRole("button", { name: "Remove web search" }));

    expect(saved).toEqual([
      {
        edits: {
          "filler.research.web_provider": "none",
          "filler.research.brave_api_key": "",
          "filler.research.searxng_url": "",
        },
      },
    ]);
  });
});
