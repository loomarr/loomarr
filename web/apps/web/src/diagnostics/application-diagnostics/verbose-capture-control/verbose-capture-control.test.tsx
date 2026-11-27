import {
  getGetDiagnosticVerboseCaptureMockHandler,
  getStartDiagnosticVerboseCaptureMockHandler,
  getStopDiagnosticVerboseCaptureMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";
import { type ApplicationFilters, DEFAULT_APPLICATION_FILTERS } from "../../application-filters";
import { VerboseCaptureControl } from "./verbose-capture-control";

const renderControl = (filters: ApplicationFilters = DEFAULT_APPLICATION_FILTERS) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <VerboseCaptureControl filters={filters} />
    </QueryClientProvider>,
  );
};

describe("VerboseCaptureControl", () => {
  it("starts a capture scoped to the current subsystem and Channel", async () => {
    let body: unknown;
    let active = false;
    server.use(
      getGetDiagnosticVerboseCaptureMockHandler(() =>
        active ? { active: true, endsAt: 1_780_000_300_000 } : { active: false },
      ),
      getStartDiagnosticVerboseCaptureMockHandler(async ({ request }) => {
        body = await request.json();
        active = true;
        return { active: true, endsAt: 1_780_000_300_000 };
      }),
    );
    renderControl({ ...DEFAULT_APPLICATION_FILTERS, subsystem: " playout ", channelId: "channel-1" });

    expect(screen.getByText(/for playout on Channel channel-1/)).toBeInTheDocument();
    await userEvent.selectOptions(
      screen.getByRole("combobox", { name: "Verbose capture duration" }),
      "10 minutes",
    );
    const start = screen.getByRole("button", { name: "Start capture" });
    await waitFor(() => expect(start).toBeEnabled());
    await userEvent.click(start);

    await waitFor(() =>
      expect(body).toEqual({ durationMinutes: 10, subsystem: "playout", channelId: "channel-1" }),
    );
    expect(await screen.findByText(/Active until/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Stop" })).toBeInTheDocument();
  });

  it("stops an active capture", async () => {
    let active = true;
    server.use(
      getGetDiagnosticVerboseCaptureMockHandler(() =>
        active ? { active: true, endsAt: 1_780_000_300_000 } : { active: false },
      ),
      getStopDiagnosticVerboseCaptureMockHandler(() => {
        active = false;
        return { active: false };
      }),
    );
    renderControl();

    await userEvent.click(await screen.findByRole("button", { name: "Stop" }));
    expect(await screen.findByRole("button", { name: "Start capture" })).toBeInTheDocument();
  });

  it("disables starting when the capture state cannot be read", async () => {
    server.use(http.get("*/v1/diagnostics/verbose-capture", () => HttpResponse.json({}, { status: 500 })));
    renderControl();

    expect(await screen.findByText("Verbose capture is unavailable.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Start capture" })).toBeDisabled();
  });
});
