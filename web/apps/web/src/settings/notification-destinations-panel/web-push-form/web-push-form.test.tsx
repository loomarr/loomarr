import type { NotificationProviderTypeDTO } from "@loomarr/api/models/notificationProviderTypeDTO";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { render } from "@/test/render";
import { WebPushForm } from "./web-push-form";

const definition: NotificationProviderTypeDTO = {
  type: "web_push",
  name: "Browser Push",
  memberOwned: true,
  events: ["proposal_approved", "channel_live"],
  fields: [],
};

const draw = (props: Partial<Parameters<typeof WebPushForm>[0]> = {}) => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  render(
    <WebPushForm definition={definition} onCancel={vi.fn()} onSave={onSave} pending={false} {...props} />,
  );
  return { onSave };
};

afterEach(() => {
  Reflect.deleteProperty(navigator, "serviceWorker");
});

describe("WebPushForm", () => {
  it("says it asks the browser for permission only when it is enabled", () => {
    draw();
    expect(screen.getByRole("heading", { name: "Add Browser Push" })).toBeInTheDocument();
    expect(screen.getByText(/asks this browser for permission only when you enable it/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Enable this browser" })).toBeInTheDocument();
  });

  it("explains that the browser cannot do it, and saves nothing, when push is unsupported", async () => {
    const { onSave } = draw({ publicKey: "B".repeat(87) });
    await userEvent.click(screen.getByRole("checkbox", { name: "Proposal Approved" }));

    await userEvent.click(screen.getByRole("button", { name: "Enable this browser" }));

    expect(await screen.findByText(/not available in this browser or connection/)).toBeInTheDocument();
    expect(onSave).not.toHaveBeenCalled();
  });

  it("saves an existing browser's label and events without touching the browser", async () => {
    const provider = {
      id: "p1",
      type: "web_push",
      label: "Kitchen laptop",
      events: ["channel_live"],
      enabled: true,
      settings: [],
      createdAt: "2026-08-31T18:00:00Z",
      updatedAt: "2026-08-31T18:00:00Z",
    };
    const { onSave } = draw({ provider });
    expect(screen.getByRole("heading", { name: "Edit Kitchen laptop" })).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Save provider" }));

    await waitFor(() =>
      expect(onSave).toHaveBeenCalledWith({
        label: "Kitchen laptop",
        events: ["channel_live"],
        enabled: true,
        settings: {},
      }),
    );
  });
});
