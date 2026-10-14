import { getChannelUpcomingMockHandler } from "@loomarr/api/msw";
import { formatEpgTime } from "@loomarr/core/format";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { ChannelUpcoming } from "./channel-upcoming";

const wrapper = ({ children }: { children: ReactNode }) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
};

describe("ChannelUpcoming", () => {
  it.each([false, true])(
    "does not claim an empty backend response proves on-air state (live=%s)",
    async (live) => {
      server.use(getChannelUpcomingMockHandler({ upcoming: [] }));

      render(<ChannelUpcoming channelId="ch-1" live={live} />, { wrapper });

      expect(await screen.findByText("Programme information isn't available.")).toBeInTheDocument();
      expect(screen.queryByText("Not on the air yet.")).not.toBeInTheDocument();
      expect(screen.queryByText("Nothing scheduled right now.")).not.toBeInTheDocument();
    },
  );

  it("badges the show airing now and keeps its start time, as the web mock's What's on does", async () => {
    const start = Date.now() - 10 * 60_000;
    server.use(
      getChannelUpcomingMockHandler({
        upcoming: [
          { gap: false, startMs: start, stopMs: start + 3_600_000, title: "Airing show" },
          { gap: false, startMs: start + 3_600_000, stopMs: start + 7_200_000, title: "Later show" },
        ],
      }),
    );

    render(<ChannelUpcoming channelId="ch-1" live />, { wrapper });

    const airing = (await screen.findByText("Airing show")).closest("li");
    expect(airing).toHaveTextContent(formatEpgTime(start));
    expect(airing).toHaveTextContent("Now");
    expect(screen.getByText("Later show").closest("li")).not.toHaveTextContent("Now");
  });

  it("badges nothing on a channel that isn't live", async () => {
    const start = Date.now() - 10 * 60_000;
    server.use(
      getChannelUpcomingMockHandler({
        upcoming: [{ gap: false, startMs: start, stopMs: start + 3_600_000, title: "Airing show" }],
      }),
    );

    render(<ChannelUpcoming channelId="ch-1" live={false} />, { wrapper });

    expect((await screen.findByText("Airing show")).closest("li")).not.toHaveTextContent("Now");
  });
});
