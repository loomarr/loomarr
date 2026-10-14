import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import type { SettingsListOutputBody } from "@loomarr/api/models/settingsListOutputBody";
import {
  getGetChannelWatermarkOptionsMockHandler,
  getSettingsListMockHandler,
  getUploadChannelWatermarkMockHandler,
} from "@loomarr/api/msw";
import { LoomarrProvider } from "@loomarr/design-system";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { toast } from "sonner";
import { describe, expect, it, vi } from "vitest";
import { channel } from "@/test/fixtures/channels";
import { server } from "@/test/msw/server";
import { ChannelOnScreenLogo } from "./channel-on-screen-logo";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

// The wiring, not the section: the holidays, era and automatic name are the backend's
// (GET …/watermark/options), the preview's drawable header drives the warning, and a picture choice
// is one PATCH of policy.watermark that keeps the rest of the channel's policy.
describe("ChannelOnScreenLogo", () => {
  it("shows the backend's options and saves a holiday picture into policy.watermark", async () => {
    server.use(
      getGetChannelWatermarkOptionsMockHandler({
        autoCallsign: "SC",
        era: "1980s",
        eraPalette: ["red", "amber", "cyan"],
        holidays: [
          {
            id: "halloween",
            name: "Halloween",
            start: "2026-10-01",
            end: "2026-10-31",
            active: true,
            picture: "pumpkin",
            pictures: [
              { id: "pumpkin", name: "Pumpkin" },
              { id: "bat", name: "Bat" },
            ],
          },
        ],
      }),
      getSettingsListMockHandler({ features: {} as SettingsListOutputBody["features"], settings: [] }),
      http.get("*/v1/channels/:id/watermark/preview", () =>
        HttpResponse.arrayBuffer(new ArrayBuffer(8), {
          headers: { "Content-Type": "image/png", "X-Loomarr-Watermark-Drawable": "false" },
        }),
      ),
    );
    const policy = { seasonal: { holidays: ["halloween"] }, watermark: { look: "tint" as const } };
    const savePolicy = vi.fn();
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <LoomarrProvider theme="dark">
          <ChannelOnScreenLogo
            channel={channel({ id: "ch-1", policy })}
            onOpenProgramming={vi.fn()}
            savePolicy={savePolicy}
          />
        </LoomarrProvider>
      </QueryClientProvider>,
    );

    expect(await screen.findByText("Oct 1 – Oct 31")).toBeInTheDocument();
    expect(screen.getByText("In effect now")).toBeInTheDocument();
    expect(screen.getByText("1980s palette for this channel.")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Auto: SC")).toBeInTheDocument();
    expect(screen.getByText(/it uses Halloween\)/)).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByText(/This server can't draw the logo right now/)).toBeInTheDocument(),
    );

    await userEvent.click(screen.getByRole("radio", { name: "Bat for Halloween" }));
    expect(savePolicy).toHaveBeenCalledWith({
      ...policy,
      watermark: { look: "tint", motifs: { halloween: "bat" } },
    });
  });

  // Upload tests: the section's "Upload an image" button opens the hidden file input; the helper
  // drives that input directly, with the options/settings/preview the section needs to render.
  const renderForUpload = (policy: ChannelPolicy, savePolicy: (p: ChannelPolicy) => void) => {
    server.use(
      getGetChannelWatermarkOptionsMockHandler({
        autoCallsign: "SC",
        era: "1980s",
        eraPalette: [],
        holidays: [],
      }),
      getSettingsListMockHandler({ features: {} as SettingsListOutputBody["features"], settings: [] }),
      http.get("*/v1/channels/:id/watermark/preview", () =>
        HttpResponse.arrayBuffer(new ArrayBuffer(8), { headers: { "Content-Type": "image/png" } }),
      ),
    );
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const ui = (p: ChannelPolicy) => (
      <QueryClientProvider client={client}>
        <LoomarrProvider theme="dark">
          <ChannelOnScreenLogo
            channel={channel({ id: "ch-1", policy: p })}
            onOpenProgramming={vi.fn()}
            savePolicy={savePolicy}
          />
        </LoomarrProvider>
      </QueryClientProvider>
    );
    const view = render(ui(policy));
    return { rerender: (p: ChannelPolicy) => view.rerender(ui(p)) };
  };
  const pickFile = async () => {
    await screen.findByRole("button", { name: "Upload an image" });
    const input = document.querySelector<HTMLInputElement>('input[type="file"]');
    if (!input) throw new Error("no file input");
    await userEvent.upload(input, new File(["x"], "logo.png", { type: "image/png" }));
  };

  it("keeps an edit made while the upload was in flight", async () => {
    let finish: () => void = () => {};
    const gate = new Promise<void>((resolve) => {
      finish = resolve;
    });
    const savePolicy = vi.fn();
    const start: ChannelPolicy = { watermark: { look: "text" } };
    const { rerender } = renderForUpload(start, savePolicy);
    server.use(
      getUploadChannelWatermarkMockHandler(async () => {
        await gate;
        return { image: "img-1" };
      }),
    );
    await pickFile();
    // The household changes the look while the file is still uploading; the page re-renders with it.
    rerender({ watermark: { look: "tint" } });
    finish();
    await waitFor(() => expect(savePolicy).toHaveBeenCalledTimes(1));
    expect(savePolicy).toHaveBeenCalledWith({ watermark: { look: "tint", image: "img-1" } });
  });

  it("shows an error and saves nothing when the upload fails", async () => {
    const savePolicy = vi.fn();
    renderForUpload({ watermark: { look: "text" } }, savePolicy);
    server.use(
      http.post("*/v1/channels/:id/watermark", () =>
        HttpResponse.json({ title: "That image is too large", status: 413 }, { status: 413 }),
      ),
    );
    await pickFile();
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("That image is too large"));
    expect(savePolicy).not.toHaveBeenCalled();
  });
});
