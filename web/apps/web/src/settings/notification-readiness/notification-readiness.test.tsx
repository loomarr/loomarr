import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { NotificationReadiness } from "./notification-readiness";

const liveValues = (values: Record<string, string>) => (key: string) => values[key] ?? "";

describe("NotificationReadiness", () => {
  it("separates link readiness from optional email delivery", () => {
    render(<NotificationReadiness liveValue={liveValues({})} />);

    expect(screen.getByText("Recipient address needed")).toBeInTheDocument();
    expect(screen.getByText("Email is off")).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "Invitation links need configuration" })).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "Account email: Email is off" })).toBeInTheDocument();
  });

  it("reports incomplete enabled email without overstating delivery", () => {
    render(
      <NotificationReadiness
        liveValue={liveValues({
          "access.public_url": "https://loomarr.example.com",
          "notifications.email.enabled": "true",
          "notifications.smtp.port": "587",
        })}
      />,
    );

    expect(screen.getByText("Ready to share")).toBeInTheDocument();
    expect(screen.getByText("Setup incomplete")).toBeInTheDocument();
  });

  it("calls account email ready only when the required delivery settings are complete", () => {
    render(
      <NotificationReadiness
        liveValue={liveValues({
          "access.public_url": "https://loomarr.example.com",
          "notifications.email.enabled": "true",
          "notifications.smtp.host": "smtp.example.com",
          "notifications.smtp.port": "587",
          "notifications.email.from_address": "loomarr@example.com",
        })}
      />,
    );

    expect(screen.getByText("Ready to test")).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "Account email: Ready to test" })).toBeInTheDocument();
  });
});
