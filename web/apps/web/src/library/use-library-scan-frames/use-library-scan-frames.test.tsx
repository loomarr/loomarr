import * as libraryApi from "@loomarr/api/endpoints/library";
import { getLibraryStatusMockHandler } from "@loomarr/api/msw";
import { unwrap } from "@loomarr/api/unwrap";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { describe, expect, it } from "vitest";
import { LoomarrEventsProvider } from "@/events/events-provider";
import { FakeEventSource } from "@/test/fake-event-source";
import { libraryStatus } from "@/test/fixtures/library";
import { server } from "@/test/msw/server";
import { useLibraryScanFrames } from "./use-library-scan-frames";

const wrapper = ({ children }: PropsWithChildren) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    <LoomarrEventsProvider>{children}</LoomarrEventsProvider>
  </QueryClientProvider>
);

const frame = {
  step: "reading",
  stepNumber: 2,
  done: 40,
  total: 200,
  current: "Harbour Lights / S01E02.mkv",
  etaSeconds: 300,
  paused: false,
  quiet: false,
};

// Each library_scan frame is the whole of the import's progress, written into the status the
// screens render from; a frame that leaves a field out clears it.
describe("useLibraryScanFrames", () => {
  it("writes each frame into the library status", async () => {
    server.use(getLibraryStatusMockHandler(libraryStatus()));
    const { result } = renderHook(
      () => {
        useLibraryScanFrames();
        return unwrap(libraryApi.useLibraryStatus().data);
      },
      { wrapper },
    );
    await waitFor(() => expect(result.current).toBeDefined());

    // The query cache tells its readers on a later tick, so each check waits for it.
    act(() => FakeEventSource.broadcast("library_scan", frame));
    await waitFor(() => expect(result.current?.scan).toMatchObject(frame));

    // Paused: the frame carries no time left and no current file, and the old ones go.
    act(() =>
      FakeEventSource.broadcast("library_scan", {
        ...frame,
        current: undefined,
        etaSeconds: undefined,
        paused: true,
      }),
    );
    await waitFor(() => expect(result.current?.scan.paused).toBe(true));
    expect(result.current?.scan.etaSeconds).toBeUndefined();
    expect(result.current?.scan.current).toBeUndefined();
  });
});
