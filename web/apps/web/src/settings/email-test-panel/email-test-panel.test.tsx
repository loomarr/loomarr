import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { SettingsEditsProvider } from "@/settings/settings-edits";
import { server } from "@/test/msw/server";
import { EmailTestPanel } from "./email-test-panel";

const wrapper = ({ children }: { children: ReactNode }) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <SettingsEditsProvider>{children}</SettingsEditsProvider>
    </QueryClientProvider>
  );
};

describe("EmailTestPanel", () => {
  it("sends one test message and announces the provider-safe result", async () => {
    let destination = "";
    server.use(
      http.post("*/v1/notifications/email/test", async ({ request }) => {
        const body = (await request.json()) as { to: string };
        destination = body.to;
        return HttpResponse.json({ ok: true, hint: "Test message accepted by the SMTP server." });
      }),
    );
    render(<EmailTestPanel />, { wrapper });

    await userEvent.type(screen.getByLabelText("Test recipient"), "admin@example.com");
    await userEvent.click(screen.getByRole("button", { name: "Send test email" }));

    expect(await screen.findByRole("status")).toHaveTextContent("accepted by the SMTP server");
    expect(destination).toBe("admin@example.com");
  });

  it("keeps provider failures actionable without rendering raw details", async () => {
    server.use(
      http.post("*/v1/notifications/email/test", () =>
        HttpResponse.json({
          ok: false,
          outcome: "configuration_invalid",
          hint: "Complete the SMTP server, security, authentication, and sender settings, then try again.",
        }),
      ),
    );
    render(<EmailTestPanel />, { wrapper });

    await userEvent.type(screen.getByLabelText("Test recipient"), "admin@example.com");
    await userEvent.click(screen.getByRole("button", { name: "Send test email" }));

    const status = await screen.findByRole("status");
    expect(status).toHaveTextContent("Complete the SMTP server");
    expect(status).not.toHaveTextContent(/password|secret|535/i);
  });
});
