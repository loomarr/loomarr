import { getSystemRestartCostMockHandler, getSystemServicesMockHandler } from "@loomarr/api/msw";
import { screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { ADMIN, renderAt, stub } from "@/test/requests-harness";

afterEach(() => vi.restoreAllMocks());

// The old Dashboard's operator panels, on their new route (#1659, Q-H7).
describe("ServerStatus on Diagnostics › Health", () => {
  it("shows the connected services, recent activity and the restart control", async () => {
    stub({ me: ADMIN });
    server.use(
      getSystemServicesMockHandler({ loomarr: { name: "loomarr", ok: true }, rows: [] }),
      getSystemRestartCostMockHandler({
        available: true,
        pendingKeys: [],
        restartRequired: false,
        streamingChannels: 0,
      }),
    );
    renderAt("/settings/system/diagnostics?view=health");
    expect(await screen.findByRole("heading", { name: "Services" })).toBeInTheDocument();
    expect(await screen.findByRole("heading", { name: "Recent activity" })).toBeInTheDocument();
    expect(await screen.findByRole("heading", { name: "Restart Loomarr" })).toBeInTheDocument();
  });
});
