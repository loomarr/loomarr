import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WebhookStep } from "./webhook-step";

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const stub = (opts: { lastReceived?: Record<string, string> }) =>
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => {
      const u = String(url);
      if (u.includes("/v1/settings/secrets/webhook_secret")) {
        return Promise.resolve(json({ value: "s3cr3t", displayable: true }));
      }
      if (u.includes("/v1/setup/status")) {
        return Promise.resolve(
          json({ checks: [{ name: "webhook", ok: false, lastReceived: opts.lastReceived ?? {} }] }),
        );
      }
      return Promise.resolve(json({}));
    }),
  );

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    {children}
  </QueryClientProvider>
);

afterEach(() => vi.restoreAllMocks());

describe("WebhookStep", () => {
  it("builds the paste-able URL from the revealed secret", async () => {
    stub({});
    render(<WebhookStep />, { wrapper });
    // The whole point of the reveal route: show what's already configured, not a new value.
    expect(await screen.findByText(/\/hooks\/arr\?token=s3cr3t/)).toBeInTheDocument();
  });

  it("listens per app, and flips only the one that reported", async () => {
    stub({ lastReceived: { sonarr: new Date().toISOString() } });
    render(<WebhookStep />, { wrapper });

    expect(await screen.findByText(/last received/i)).toBeInTheDocument();
    // Radarr hasn't reported — it must still read as listening, not as failed.
    expect(screen.getByText(/press Test in this app/i)).toBeInTheDocument();
  });
});
