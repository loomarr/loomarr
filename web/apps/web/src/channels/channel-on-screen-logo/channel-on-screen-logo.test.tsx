import type { SettingsListOutputBody } from "@loomarr/api/models/settingsListOutputBody";
import { getGetChannelWatermarkOptionsMockHandler, getSettingsListMockHandler } from "@loomarr/api/msw";
import { LoomarrProvider } from "@loomarr/design-system";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
import { channel } from "@/test/fixtures/channels";
import { server } from "@/test/msw/server";
import { ChannelOnScreenLogo } from "./channel-on-screen-logo";

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
});
