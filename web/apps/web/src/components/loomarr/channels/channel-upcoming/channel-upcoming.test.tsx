import { getChannelUpcomingMockHandler } from "@loomarr/api/msw";
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
});
