import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { NotificationDestinationsPanel } from "./notification-destinations-panel";

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider
    client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}
  >
    {children}
  </QueryClientProvider>
);

const destination = {
  id: "destination-1",
  means: "slack",
  label: "Operations Slack",
  scope: "installation",
  audience: "operators",
  topics: ["channel_degraded"],
  enabled: true,
  credentialsConfigured: true,
  createdAt: "2026-08-31T18:00:00Z",
  updatedAt: "2026-08-31T18:00:00Z",
  health: {
    lastSuccessAt: "2026-08-31T19:00:00Z",
    lastFailureAt: "2026-08-31T20:00:00Z",
    lastFailureOutcome: "transport_unavailable",
    queuedCount: 2,
    terminalFailureCount: 1,
  },
};

describe("NotificationDestinationsPanel", () => {
  it("renders redacted health and queues a test without claiming final delivery", async () => {
    let requestID = "";
    server.use(
      http.get("*/v1/notifications/destinations", () => HttpResponse.json({ destinations: [destination] })),
      http.post("*/v1/notifications/destinations/destination-1/test", async ({ request }) => {
        requestID = ((await request.json()) as { requestId: string }).requestId;
        return HttpResponse.json(
          {
            intentId: "intent-test-1",
            queued: true,
            hint: "Test notification queued. Check delivery health for the final provider result.",
          },
          { status: 202 },
        );
      }),
    );

    render(<NotificationDestinationsPanel scope="installation" />, { wrapper });

    expect(await screen.findByText("Operations Slack")).toBeInTheDocument();
    expect(screen.getByText("2 queued · 1 failed")).toBeInTheDocument();
    expect(screen.getByText(/transport unavailable/i)).toBeInTheDocument();
    expect(document.body).not.toHaveTextContent(/token|credential value|payload/i);

    await userEvent.click(screen.getByRole("button", { name: "Test Operations Slack" }));
    expect(await screen.findByRole("status")).toHaveTextContent("queued");
    expect(screen.getByRole("status")).not.toHaveTextContent(/delivered|sent successfully/i);
    expect(requestID).not.toBe("");
  });

  it("creates a disabled provider draft with selected audience-compatible events", async () => {
    let created: Record<string, unknown> | undefined;
    server.use(
      http.get("*/v1/notifications/destinations", () => HttpResponse.json({ destinations: [] })),
      http.post("*/v1/notifications/destinations", async ({ request }) => {
        created = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({ ...destination, ...created, id: "destination-2" }, { status: 201 });
      }),
    );
    render(<NotificationDestinationsPanel scope="installation" />, { wrapper });

    await userEvent.type(await screen.findByLabelText("Destination label"), "Living room alerts");
    await userEvent.selectOptions(screen.getByLabelText("Provider"), "slack");
    await userEvent.selectOptions(screen.getByLabelText("Audience"), "operators");
    await userEvent.click(screen.getByLabelText("Channel degraded"));
    await userEvent.click(screen.getByRole("button", { name: "Create destination draft" }));

    await waitFor(() =>
      expect(created).toEqual({
        means: "slack",
        label: "Living room alerts",
        scope: "installation",
        audience: "operators",
        topics: ["channel_degraded"],
        enabled: false,
      }),
    );
  });

  it("updates policy without sending write-only provider fields and can delete", async () => {
    let update: Record<string, unknown> | undefined;
    let deleted = false;
    server.use(
      http.get("*/v1/notifications/destinations", () => HttpResponse.json({ destinations: [destination] })),
      http.put("*/v1/notifications/destinations/destination-1", async ({ request }) => {
        update = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({ ...destination, ...update });
      }),
      http.delete("*/v1/notifications/destinations/destination-1", () => {
        deleted = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    render(<NotificationDestinationsPanel scope="installation" />, { wrapper });

    await userEvent.click(await screen.findByRole("button", { name: "Edit Operations Slack" }));
    await userEvent.clear(screen.getByLabelText("Edit label"));
    await userEvent.type(screen.getByLabelText("Edit label"), "On-call Slack");
    const row = screen.getByRole("heading", { name: "On-call Slack" }).closest("li");
    if (!row) throw new Error("destination row missing");
    await userEvent.click(within(row).getByLabelText("Acquisition gave up"));
    await userEvent.click(screen.getByRole("button", { name: "Save On-call Slack" }));

    await waitFor(() => expect(update?.label).toBe("On-call Slack"));
    expect(update).not.toHaveProperty("configuration");
    expect(update).not.toHaveProperty("credentials");

    await userEvent.click(screen.getByRole("button", { name: "Delete On-call Slack" }));
    await waitFor(() => expect(deleted).toBe(true));
  });
});
