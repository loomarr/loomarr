import type { NotificationProviderDTO } from "@loomarr/api/models/notificationProviderDTO";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";
import { ProviderRow } from "./provider-row";

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider
    client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}
  >
    {children}
  </QueryClientProvider>
);

const provider: NotificationProviderDTO = {
  id: "provider-1",
  type: "slack",
  label: "Operations Slack",
  events: ["channel_degraded", "acquisition_gave_up"],
  enabled: true,
  settings: [{ key: "webhookUrl", secretConfigured: true }],
  createdAt: "2026-08-31T18:00:00Z",
  updatedAt: "2026-08-31T18:00:00Z",
  health: {
    lastFailureAt: "2026-08-31T20:00:00Z",
    lastFailureOutcome: "transport_unavailable",
    queuedCount: 2,
    terminalFailureCount: 1,
  },
};

const draw = (over: Partial<NotificationProviderDTO> = {}) =>
  render(
    <ul>
      <ProviderRow provider={{ ...provider, ...over }} />
    </ul>,
    { wrapper },
  );

describe("ProviderRow", () => {
  it("shows the provider's state, events and health in words", () => {
    draw();
    expect(screen.getByRole("heading", { name: "Operations Slack" })).toBeInTheDocument();
    expect(screen.getByText("Enabled")).toBeInTheDocument();
    expect(screen.getByText("Channel Degraded, Acquisition Gave Up")).toBeInTheDocument();
    expect(screen.getByText(/2 queued · 1 failed · Last failure: Transport Unavailable/)).toBeInTheDocument();
    expect(screen.getByText("Slack")).toBeInTheDocument();
  });

  it("offers Enable on a disabled provider and turns the Send test button off", () => {
    draw({ enabled: false });
    expect(screen.getByText("Disabled")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Enable" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Send test" })).toBeDisabled();
  });

  it("arms Delete provider only once its name is typed back, and Cancel puts the actions back", async () => {
    draw();
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(screen.getByText("This can't be undone.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Edit" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Delete provider" })).toBeDisabled();

    await userEvent.type(screen.getByLabelText("Type Operations Slack to confirm"), "Operations Slack");
    expect(screen.getByRole("button", { name: "Delete provider" })).toBeEnabled();

    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByRole("button", { name: "Edit" })).toBeInTheDocument();
  });

  it("says so when the provider is switched off", async () => {
    let body: unknown;
    server.use(
      http.patch("*/v1/notifications/providers/provider-1", async ({ request }) => {
        body = await request.json();
        return HttpResponse.json({ ...provider, enabled: false });
      }),
      http.put("*/v1/notifications/providers/provider-1", async ({ request }) => {
        body = await request.json();
        return HttpResponse.json({ ...provider, enabled: false });
      }),
    );
    draw();

    await userEvent.click(screen.getByRole("button", { name: "Disable" }));

    await waitFor(() => expect(screen.getByText("Provider saved.")).toBeInTheDocument());
    expect(body).toMatchObject({ enabled: false, label: "Operations Slack" });
  });
});
