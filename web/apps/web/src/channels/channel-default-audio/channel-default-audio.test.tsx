import { getChannelTracksMockHandler } from "@loomarr/api/msw";
import { LoomarrProvider } from "@loomarr/design-system";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { channel } from "@/test/fixtures/channels";
import { server } from "@/test/msw/server";
import { ChannelDefaultAudio } from "./channel-default-audio";

const renderCard = (props: Parameters<typeof ChannelDefaultAudio>[0]) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <LoomarrProvider theme="dark">
        <ChannelDefaultAudio {...props} />
      </LoomarrProvider>
    </QueryClientProvider>,
  );
};

// The wiring: the choices are the airing media's real tracks (GET …/tracks), and a choice is one save
// of policy.playout.audioLanguage that keeps the rest of the channel's policy.
describe("ChannelDefaultAudio", () => {
  it("offers the airing tracks and saves the choice into policy.playout, keeping the rest", async () => {
    server.use(
      getChannelTracksMockHandler({
        audio: [
          { index: 0, language: "eng" },
          { index: 1, language: "fra" },
        ],
        subtitles: [],
      }),
    );
    const savePolicy = vi.fn();
    const policy = { watermark: { look: "tint" as const }, playout: {} };
    renderCard({ channel: channel({ status: "live", policy }), savePolicy });

    await userEvent.click(screen.getByRole("button", { name: /^Default audio language/ }));
    await userEvent.click(await screen.findByRole("radio", { name: /French/ }));

    expect(savePolicy).toHaveBeenCalledWith({ ...policy, playout: { audioLanguage: "fra" } });
  });

  it("keeps a saved language the airing programme doesn't carry, and Auto saves as empty", async () => {
    server.use(getChannelTracksMockHandler({ audio: [{ index: 0, language: "eng" }], subtitles: [] }));
    const savePolicy = vi.fn();
    renderCard({
      channel: channel({ status: "live", policy: { playout: { audioLanguage: "jpn" } } }),
      savePolicy,
    });

    const picker = screen.getByRole("button", { name: /^Default audio language/ });
    expect(picker).toHaveTextContent("JPN");
    await userEvent.click(picker);
    await userEvent.click(await screen.findByRole("radio", { name: /^Auto \(default\)/ }));

    expect(savePolicy).toHaveBeenCalledWith({ playout: { audioLanguage: "" } });
  });
});
