import type { NotificationProviderTypeDTO } from "@loomarr/api/models/notificationProviderTypeDTO";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { render } from "@/test/render";
import { ProviderForm } from "./provider-form";

const definition: NotificationProviderTypeDTO = {
  type: "email",
  name: "SMTP",
  memberOwned: false,
  events: ["proposal_submitted", "channel_degraded"],
  fields: [
    { key: "host", label: "SMTP host", kind: "text", required: true, sensitive: false },
    { key: "password", label: "Password", kind: "password", required: false, sensitive: true },
  ],
};

const draw = (props: Partial<Parameters<typeof ProviderForm>[0]> = {}) => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  const onCancel = vi.fn();
  render(
    <ProviderForm definition={definition} onCancel={onCancel} onSave={onSave} pending={false} {...props} />,
  );
  return { onCancel, onSave };
};

describe("ProviderForm", () => {
  it("names the provider type it adds, starting from the type's own name as the label", () => {
    draw();
    expect(screen.getByRole("heading", { name: "Add SMTP" })).toBeInTheDocument();
    expect(screen.getByLabelText("Label *")).toHaveValue("SMTP");
  });

  it("saves the trimmed label, the typed settings and the chosen events", async () => {
    const { onSave } = draw();
    await userEvent.clear(screen.getByLabelText("Label *"));
    await userEvent.type(screen.getByLabelText("Label *"), "  Mail  ");
    await userEvent.type(screen.getByLabelText(/SMTP host/), "mail.example.test");
    await userEvent.click(screen.getByRole("checkbox", { name: "Channel Degraded" }));

    await userEvent.click(screen.getByRole("button", { name: "Save provider" }));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    expect(onSave).toHaveBeenCalledWith({
      label: "Mail",
      events: ["channel_degraded"],
      enabled: true,
      settings: { host: "mail.example.test" },
    });
  });

  it("holds a save back until the required settings and one event are there", async () => {
    const { onSave } = draw();

    await userEvent.click(screen.getByRole("button", { name: "Save provider" }));

    expect(await screen.findAllByRole("alert")).not.toHaveLength(0);
    expect(onSave).not.toHaveBeenCalled();
  });

  it("sends an empty string for a secret the person chose to clear, and leaves a kept one out", async () => {
    const provider = {
      id: "p1",
      type: "email",
      label: "Mail",
      events: ["channel_degraded"],
      enabled: true,
      settings: [
        { key: "host", value: "mail.example.test", secretConfigured: false },
        { key: "password", secretConfigured: true },
      ],
      createdAt: "2026-08-31T18:00:00Z",
      updatedAt: "2026-08-31T18:00:00Z",
    };
    const { onSave } = draw({ provider });
    expect(screen.getByRole("heading", { name: "Edit Mail" })).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Save provider" }));
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    expect(onSave.mock.calls[0]?.[0].settings).toEqual({ host: "mail.example.test" });

    await userEvent.click(screen.getByRole("button", { name: "Clear Password" }));
    await userEvent.click(screen.getByRole("button", { name: "Save provider" }));
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(2));
    expect(onSave.mock.calls[1]?.[0].settings).toEqual({ host: "mail.example.test", password: "" });
  });

  it("closes from the header button and from Cancel", async () => {
    const { onCancel } = draw();
    await userEvent.click(screen.getByRole("button", { name: "Close provider form" }));
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCancel).toHaveBeenCalledTimes(2);
  });
});
