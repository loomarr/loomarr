import { getChannelGuideMockHandler } from "@loomarr/api/msw";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { ADMIN, MEMBER, renderAt, stub } from "@/test/requests-harness";

// The top of Home through the real route (#1822 evidence: "Neutral channel inventory count").
// Services, requests needing a decision and restart-pending now live elsewhere (the Requests nav
// badge, Settings → This server's Diagnostics), so this strip only ever says one of: loading, a
// failure to retry, the first-channel invitation, or a plain count.

afterEach(() => vi.restoreAllMocks());

const timeline = (id: string) => ({
  airings: [],
  channelId: id,
  name: `Channel ${id}`,
  number: 1,
  pendingCount: 0,
  status: "live" as const,
});

describe("HomeStrip — a neutral inventory line (#1822)", () => {
  it("says how many channels there are", async () => {
    stub({ me: ADMIN });
    server.use(
      getChannelGuideMockHandler({ channels: [timeline("c1"), timeline("c2")], fromMs: 0, toMs: 0 }),
    );
    renderAt("/dashboard");
    expect(await screen.findByText("2 channels")).toBeInTheDocument();
  });

  it("uses the singular for one channel", async () => {
    stub({ me: MEMBER });
    server.use(getChannelGuideMockHandler({ channels: [timeline("c1")], fromMs: 0, toMs: 0 }));
    renderAt("/dashboard");
    expect(await screen.findByText("1 channel")).toBeInTheDocument();
  });

  it("shows a stable loading line while the guide hasn't answered", async () => {
    stub({ me: ADMIN });
    server.use(getChannelGuideMockHandler(() => new Promise(() => {})));
    renderAt("/dashboard");
    expect(await screen.findByText("Loading your channels…")).toBeInTheDocument();
  });

  it("offers Try again on an initial error, and retries the guide", async () => {
    stub({ me: ADMIN });
    let calls = 0;
    server.use(
      http.get("*/v1/guide", () => {
        calls += 1;
        return HttpResponse.json({ title: "Guide unavailable" }, { status: 500 });
      }),
    );
    renderAt("/dashboard");
    expect(await screen.findByText("Couldn’t load your channels.")).toBeInTheDocument();
    const before = calls;
    await userEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(calls).toBeGreaterThan(before);
  });

  it("offers the admin a creation action on zero channels", async () => {
    stub({ me: ADMIN });
    server.use(getChannelGuideMockHandler({ channels: [], fromMs: 0, toMs: 0 }));
    renderAt("/dashboard");
    expect(await screen.findByText("Your first channel starts here")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Add your first channel" })).toBeInTheDocument();
  });

  it("offers the member a request action on zero channels", async () => {
    stub({ me: MEMBER });
    server.use(getChannelGuideMockHandler({ channels: [], fromMs: 0, toMs: 0 }));
    renderAt("/dashboard");
    expect(await screen.findByText("Your first channel starts here")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Request a channel" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Add your first channel" })).not.toBeInTheDocument();
  });
});
