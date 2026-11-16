import type { NotificationProviderFieldDTO } from "@loomarr/api/models/notificationProviderFieldDTO";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { render } from "@/test/render";
import { ProviderFieldInput } from "./provider-field-input";

const field = (over: Partial<NotificationProviderFieldDTO>): NotificationProviderFieldDTO => ({
  key: "host",
  kind: "text",
  label: "SMTP host",
  required: true,
  sensitive: false,
  ...over,
});

const draw = (
  over: Partial<NotificationProviderFieldDTO>,
  props: Partial<Parameters<typeof ProviderFieldInput>[0]> = {},
) => {
  const onChange = vi.fn();
  const onClear = vi.fn();
  render(
    <ProviderFieldInput
      cleared={false}
      configured={false}
      field={field(over)}
      inputId="host"
      onChange={onChange}
      onClear={onClear}
      value=""
      {...props}
    />,
  );
  return { onChange, onClear };
};

describe("ProviderFieldInput", () => {
  it("marks a required text field and hands back what is typed", async () => {
    const { onChange } = draw({});
    expect(screen.getByText("SMTP host *")).toBeInTheDocument();

    await userEvent.type(screen.getByLabelText(/SMTP host/), "a");

    expect(onChange).toHaveBeenCalledWith("a");
  });

  it("draws a toggle as a checkbox whose change is the text 'true' or 'false'", async () => {
    const { onChange } = draw({ kind: "toggle", label: "Use TLS", required: false }, { value: "false" });

    await userEvent.click(screen.getByRole("checkbox", { name: "Use TLS" }));

    expect(onChange).toHaveBeenCalledWith("true");
  });

  it("keeps a saved secret and offers to clear it, then to keep it again", async () => {
    const { onClear } = draw(
      { kind: "password", label: "Password", required: false, sensitive: true },
      { configured: true },
    );
    expect(screen.getByText("Configured — leave blank to keep it.")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Clear Password" }));

    expect(onClear).toHaveBeenCalledTimes(1);
  });

  it("says a cleared secret will be cleared, disables its input and offers to keep it", () => {
    draw(
      { kind: "password", label: "Password", required: false, sensitive: true },
      { cleared: true, configured: true },
    );

    expect(screen.getByText("Will be cleared when saved.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Keep saved value" })).toBeInTheDocument();
    expect(screen.getByLabelText(/Password/)).toBeDisabled();
  });

  it("announces an error and ties it to its input", () => {
    draw({}, { error: "SMTP host is required" });

    const message = screen.getByRole("alert");
    expect(message).toHaveTextContent("SMTP host is required");
    expect(screen.getByLabelText(/SMTP host/)).toHaveAttribute("aria-describedby", "host-error");
  });

  it("shows a field's description beneath it", () => {
    draw({ description: "The server to send through." });
    expect(screen.getByText("The server to send through.")).toBeInTheDocument();
  });
});
