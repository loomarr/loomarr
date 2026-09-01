import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { NotificationDestinationsPanel } from "./notification-destinations-panel";

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider
    client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}
  >
    {children}
  </QueryClientProvider>
);

const providerTypes = [
  {
    type: "email",
    name: "SMTP",
    memberOwned: false,
    events: ["proposal_submitted", "proposal_approved", "channel_degraded"],
    fields: [
      { key: "host", label: "SMTP host", kind: "text", required: true, sensitive: false },
      { key: "password", label: "Password", kind: "password", required: false, sensitive: true },
    ],
  },
  {
    type: "slack",
    name: "Slack",
    memberOwned: false,
    events: ["proposal_submitted", "acquisition_gave_up", "channel_degraded"],
    fields: [{ key: "webhookUrl", label: "Slack webhook URL", kind: "url", required: true, sensitive: true }],
  },
  {
    type: "web_push",
    name: "Browser Push",
    memberOwned: true,
    events: ["proposal_approved", "proposal_declined", "channel_live"],
    fields: [
      { key: "endpoint", label: "Push endpoint", kind: "password", required: true, sensitive: true },
      { key: "p256dh", label: "Browser public key", kind: "password", required: true, sensitive: true },
      {
        key: "auth",
        label: "Browser authentication secret",
        kind: "password",
        required: true,
        sensitive: true,
      },
    ],
  },
];

const provider = {
  id: "provider-1",
  type: "slack",
  label: "Operations Slack",
  events: ["channel_degraded"],
  enabled: true,
  settings: [{ key: "webhookUrl", secretConfigured: true }],
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

const providerTypeHandler = http.get("*/v1/notifications/provider-types", () =>
  HttpResponse.json({ providers: providerTypes, webPushPublicKey: "B".repeat(87) }),
);

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("NotificationDestinationsPanel", () => {
  it("shows redacted provider health and queues a test without claiming delivery", async () => {
    let requestID = "";
    server.use(
      providerTypeHandler,
      http.get("*/v1/notifications/providers", () => HttpResponse.json({ providers: [provider] })),
      http.post("*/v1/notifications/providers/provider-1/test", async ({ request }) => {
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

    render(<NotificationDestinationsPanel />, { wrapper });

    expect(await screen.findByText("Operations Slack")).toBeInTheDocument();
    expect(screen.getByText(/2 queued · 1 failed/i)).toBeInTheDocument();
    expect(screen.getByText(/transport unavailable/i)).toBeInTheDocument();
    expect(document.body).not.toHaveTextContent(/hooks\.slack|credential value|payload/i);

    await userEvent.click(screen.getByRole("button", { name: "Send test" }));
    expect(await screen.findByRole("status")).toHaveTextContent("queued");
    expect(screen.getByRole("status")).not.toHaveTextContent(/delivered|sent successfully/i);
    expect(requestID).not.toBe("");
  });

  it("adds a configured provider in the single provider-settings-events flow", async () => {
    let created: Record<string, unknown> | undefined;
    server.use(
      providerTypeHandler,
      http.get("*/v1/notifications/providers", () => HttpResponse.json({ providers: [] })),
      http.post("*/v1/notifications/providers", async ({ request }) => {
        created = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json(
          { ...provider, ...created, id: "provider-2", settings: [] },
          { status: 201 },
        );
      }),
    );
    render(<NotificationDestinationsPanel />, { wrapper });

    await userEvent.click(await screen.findByRole("button", { name: "Add provider" }));
    await userEvent.selectOptions(screen.getByLabelText("Provider"), "slack");
    await userEvent.clear(screen.getByLabelText("Label *"));
    await userEvent.type(screen.getByLabelText("Label *"), "Living room alerts");
    await userEvent.type(
      screen.getByLabelText("Slack webhook URL *"),
      "https://hooks.slack.com/services/secret",
    );
    await userEvent.click(screen.getByLabelText("Channel Degraded"));
    await userEvent.click(screen.getByRole("button", { name: "Save provider" }));

    await waitFor(() =>
      expect(created).toEqual({
        type: "slack",
        label: "Living room alerts",
        events: ["channel_degraded"],
        enabled: true,
        settings: { webhookUrl: "https://hooks.slack.com/services/secret" },
      }),
    );
  });

  it("edits ordinary choices without resending a configured secret and confirms deletion", async () => {
    let update: Record<string, unknown> | undefined;
    let deleted = false;
    server.use(
      providerTypeHandler,
      http.get("*/v1/notifications/providers", () => HttpResponse.json({ providers: [provider] })),
      http.put("*/v1/notifications/providers/provider-1", async ({ request }) => {
        update = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({ ...provider, ...update });
      }),
      http.delete("*/v1/notifications/providers/provider-1", () => {
        deleted = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    render(<NotificationDestinationsPanel />, { wrapper });

    await userEvent.click(await screen.findByRole("button", { name: "Edit" }));
    await userEvent.clear(screen.getByLabelText("Label *"));
    await userEvent.type(screen.getByLabelText("Label *"), "On-call Slack");
    await userEvent.click(screen.getByLabelText("Acquisition Gave Up"));
    await userEvent.click(screen.getByRole("button", { name: "Save provider" }));

    await waitFor(() => expect(update?.label).toBe("On-call Slack"));
    expect(update).toMatchObject({ settings: {} });
    expect(JSON.stringify(update)).not.toContain("webhookUrl");

    await userEvent.click(await screen.findByRole("button", { name: "Delete" }));
    expect(deleted).toBe(false);
    await userEvent.type(screen.getByLabelText("Type Operations Slack to confirm"), "Operations Slack");
    await userEvent.click(screen.getByRole("button", { name: "Delete provider" }));
    await waitFor(() => expect(deleted).toBe(true));
  });

  it("keeps labels associated with unique controls when add and edit forms are both open", async () => {
    server.use(
      providerTypeHandler,
      http.get("*/v1/notifications/providers", () => HttpResponse.json({ providers: [provider] })),
    );
    render(<NotificationDestinationsPanel />, { wrapper });

    await userEvent.click(await screen.findByRole("button", { name: "Add provider" }));
    await userEvent.selectOptions(screen.getByLabelText("Provider"), "slack");
    await userEvent.click(screen.getByRole("button", { name: "Edit" }));

    const labelInputs = screen.getAllByLabelText("Label *");
    expect(labelInputs).toHaveLength(2);
    expect(new Set(labelInputs.map((input) => input.id)).size).toBe(2);

    const eventInputs = screen.getAllByLabelText("Channel Degraded");
    expect(eventInputs).toHaveLength(2);
    expect(new Set(eventInputs.map((input) => input.id)).size).toBe(2);
  });

  it("validates provider fields on submit and renders provider-safe API problem details", async () => {
    server.use(
      providerTypeHandler,
      http.get("*/v1/notifications/providers", () => HttpResponse.json({ providers: [] })),
      http.post("*/v1/notifications/providers", () =>
        HttpResponse.json(
          {
            type: "https://loomarr.dev/problems/notification-provider-invalid",
            title: "Provider settings rejected",
            status: 422,
            detail: "The Slack webhook could not be accepted.",
          },
          { status: 422, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    render(<NotificationDestinationsPanel />, { wrapper });

    await userEvent.click(await screen.findByRole("button", { name: "Add provider" }));
    await userEvent.selectOptions(screen.getByLabelText("Provider"), "slack");
    await userEvent.click(screen.getByRole("button", { name: "Save provider" }));

    expect(await screen.findByText("Choose at least one event.")).toBeInTheDocument();
    expect(screen.getByText("Enter Slack webhook URL.")).toBeInTheDocument();

    await userEvent.type(
      screen.getByLabelText("Slack webhook URL *"),
      "https://hooks.slack.com/services/secret",
    );
    await userEvent.click(screen.getByLabelText("Channel Degraded"));
    await userEvent.click(screen.getByRole("button", { name: "Save provider" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Provider settings rejected");
    expect(screen.getByRole("alert")).toHaveTextContent("The Slack webhook could not be accepted.");
  });

  it("requests browser permission only from the explicit Browser Push action", async () => {
    const requestPermission = vi.fn(async () => "granted" as NotificationPermission);
    const subscribe = vi.fn(async () => ({
      toJSON: () => ({
        endpoint: "https://push.example.test/subscription/secret",
        keys: { p256dh: "browser-public-key", auth: "browser-auth-secret" },
      }),
      unsubscribe: vi.fn(async () => true),
    }));
    const register = vi.fn(async () => ({
      pushManager: { getSubscription: vi.fn(async () => null), subscribe },
    }));
    vi.stubGlobal("Notification", { permission: "default", requestPermission });
    vi.stubGlobal("PushManager", class {});
    const navigatorWithServiceWorker = Object.create(navigator) as Navigator;
    Object.defineProperty(navigatorWithServiceWorker, "serviceWorker", { value: { register } });
    vi.stubGlobal("navigator", navigatorWithServiceWorker);
    let created: Record<string, unknown> | undefined;
    server.use(
      providerTypeHandler,
      http.get("*/v1/notifications/providers", () => HttpResponse.json({ providers: [] })),
      http.post("*/v1/notifications/providers", async ({ request }) => {
        created = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({ ...provider, ...created, id: "browser-1", settings: [] }, { status: 201 });
      }),
    );
    render(<NotificationDestinationsPanel />, { wrapper });

    await userEvent.click(await screen.findByRole("button", { name: "Add provider" }));
    await userEvent.selectOptions(screen.getByLabelText("Provider"), "web_push");
    expect(requestPermission).not.toHaveBeenCalled();
    expect(screen.queryByLabelText("Push endpoint *")).not.toBeInTheDocument();
    await userEvent.click(screen.getByLabelText("Proposal Approved"));
    await userEvent.click(screen.getByRole("button", { name: "Enable this browser" }));

    await waitFor(() => expect(created).toBeDefined());
    expect(requestPermission).toHaveBeenCalledOnce();
    expect(register).toHaveBeenCalledWith("/push-worker.js");
    expect(subscribe).toHaveBeenCalledWith(expect.objectContaining({ userVisibleOnly: true }));
    expect(created).toEqual({
      type: "web_push",
      label: "This browser",
      events: ["proposal_approved"],
      enabled: true,
      settings: {
        endpoint: "https://push.example.test/subscription/secret",
        p256dh: "browser-public-key",
        auth: "browser-auth-secret",
      },
    });
  });
});
