import {
  getChannelGuideMockHandler,
  getSystemRestartCostMockHandler,
  getSystemServicesMockHandler,
} from "@loomarr/api/msw";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { ADMIN, renderAt, stub } from "@/test/requests-harness";

// The admin strip through the real route (#1659 web mock). Requests and the member strip are
// covered in test/requests-page and test/home-member.

afterEach(() => vi.restoreAllMocks());

const timeline = (status: "live" | "paused", id: string) => ({
  airings: [],
  channelId: id,
  name: `Channel ${id}`,
  number: 1,
  pendingCount: 0,
  status,
});

const strip = (over: { paused?: boolean; failing?: boolean; pendingKeys?: string[] }) => {
  stub({ me: ADMIN });
  server.use(
    getChannelGuideMockHandler({
      channels: [timeline("live", "c1"), timeline(over.paused ? "paused" : "live", "c2")],
      fromMs: 0,
      toMs: 0,
    }),
    getSystemServicesMockHandler({
      loomarr: { name: "loomarr", ok: true },
      rows: over.failing ? [{ name: "tunarr", ok: false, settingsGroup: "tunarr" }] : [],
    }),
    getSystemRestartCostMockHandler({
      available: true,
      pendingKeys: over.pendingKeys ?? [],
      restartRequired: (over.pendingKeys ?? []).length > 0,
      streamingChannels: 0,
    }),
  );
  renderAt("/dashboard");
};

describe("HomeStrip — the admin's headline", () => {
  it("says all channels are playing only when every one is live", async () => {
    strip({});
    expect(await screen.findByText("All 2 channels are playing")).toBeInTheDocument();
  });

  it("counts the live ones when a channel is paused", async () => {
    strip({ paused: true });
    expect(await screen.findByText("1 of 2 channels are playing")).toBeInTheDocument();
  });

  it("leads with a service that isn't answering, and offers Fix", async () => {
    strip({ failing: true });
    expect(await screen.findByText("Something needs fixing")).toBeInTheDocument();
    expect(screen.getByText("1 connected service isn’t answering")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Fix" })).toBeInTheDocument();
  });

  it("asks before restarting for a setting that waits on one", async () => {
    strip({ pendingKeys: ["filler.dir"] });
    await userEvent.click(await screen.findByRole("button", { name: "Restart…" }));
    expect(screen.getByText(/Restart now\? Channels pause for a few seconds/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByText(/Restart now\?/)).not.toBeInTheDocument();
  });
});
