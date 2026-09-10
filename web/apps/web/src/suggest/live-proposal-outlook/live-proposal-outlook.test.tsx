import { getGetProposalOutlookMockHandler } from "@loomarr/api/msw";
import { proposal } from "@loomarr/fixtures";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { expect, it } from "vitest";
import { outlook } from "@/test/fixtures/outlook";
import { server } from "@/test/msw/server";
import { LiveProposalOutlook } from "./live-proposal-outlook";

it("an older response cannot replace the estimate for newer approval edits", async () => {
  let releaseOld = () => {};
  const oldPending = new Promise<void>((resolve) => {
    releaseOld = resolve;
  });
  let oldReturned = false;
  const requests: unknown[] = [];
  server.use(
    getGetProposalOutlookMockHandler(async ({ request }) => {
      const edit = await request.json();
      requests.push(edit);
      if (requests.length === 1) {
        await oldPending;
        oldReturned = true;
        return outlook();
      }
      return outlook({
        state: "waiting",
        missingAcquisitions: 1,
        programs: 0,
        uniqueRuntimeMs: 0,
        firstRepeatMs: null,
      });
    }),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const view = render(
    <QueryClientProvider client={client}>
      <LiveProposalOutlook id="p1" proposal={proposal} edit={{}} />
    </QueryClientProvider>,
  );
  await waitFor(() => expect(requests).toHaveLength(1));
  expect(screen.getByRole("status")).toHaveTextContent("Checking this lineup");
  view.rerender(
    <QueryClientProvider client={client}>
      <LiveProposalOutlook id="p1" proposal={proposal} edit={{ drop: ["movie:tmdb:949"] }} />
    </QueryClientProvider>,
  );
  await screen.findByText("Waiting on 1 acquisition");
  releaseOld();
  await waitFor(() => expect(oldReturned).toBe(true));
  await waitFor(() => expect(requests).toEqual([{}, { drop: ["movie:tmdb:949"] }]));
  expect(screen.queryByText("Starts now after approval")).not.toBeInTheDocument();
  expect(screen.getByText("Waiting on 1 acquisition")).toBeInTheDocument();
  client.clear();
});
