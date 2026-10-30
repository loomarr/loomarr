import {
  getDeleteFillerSourceMockHandler,
  getFetchFillerSourceMockHandler,
  getSetFillerSourceEnabledMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { createElement, type ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { useSourceActions } from "./use-source-actions";

// One client per test file would leak cache between tests, and a client built inside the wrapper
// would be rebuilt on every render — dropping the mutation state under test.
const wrapperFor = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => createElement(QueryClientProvider, { client }, children);
};

describe("useSourceActions", () => {
  it("sends the update body for the source named, and reports which one is in flight", async () => {
    const sent: { id: string; body: unknown }[] = [];
    server.use(
      getSetFillerSourceEnabledMockHandler(async ({ request, params }) => {
        sent.push({ id: String(params.id), body: await request.json() });
        return { id: String(params.id), enabled: false };
      }),
    );
    const { result } = renderHook(() => useSourceActions({ onSourceRemoved: vi.fn() }), {
      wrapper: wrapperFor(),
    });

    expect(result.current.updatingSourceID).toBeUndefined();
    act(() => result.current.updateSource("archive:classic", { enabled: false }));

    await waitFor(() => expect(result.current.updatingSourceID).toBe("archive:classic"));
    await waitFor(() => expect(result.current.updating).toBe(false));
    expect(result.current.updatingSourceID).toBeUndefined();
    expect(sent).toEqual([{ id: "archive:classic", body: { enabled: false } }]);
  });

  it("tells the caller when a removal lands so the drawer can close", async () => {
    server.use(getDeleteFillerSourceMockHandler(() => undefined));
    const onSourceRemoved = vi.fn();
    const { result } = renderHook(() => useSourceActions({ onSourceRemoved }), { wrapper: wrapperFor() });

    act(() => result.current.removeSource("library:ads"));

    await waitFor(() => expect(onSourceRemoved).toHaveBeenCalledOnce());
  });

  it("hands a check's result to the caller, and keeps its failure for the drawer", async () => {
    server.use(
      getFetchFillerSourceMockHandler(({ request }) => ({
        sourceId: new URL(request.url).searchParams.get("id") ?? "",
        sourcesPolled: 1,
        queued: 0,
        skipped: 0,
        maxPerCheck: 10,
        total: 0,
        added: 3,
        updated: 0,
        pruned: 0,
      })),
    );
    const { result } = renderHook(() => useSourceActions({ onSourceRemoved: vi.fn() }), {
      wrapper: wrapperFor(),
    });
    const onResult = vi.fn();

    act(() => result.current.fetchSource.run("folder", onResult));
    await waitFor(() => expect(onResult).toHaveBeenCalledOnce());
    expect(onResult.mock.calls[0]?.[0]).toMatchObject({ sourceId: "folder", added: 3 });
    expect(result.current.fetchSource.error).toBeNull();

    server.use(
      http.post("*/v1/filler/sources/fetch", () =>
        HttpResponse.json({ title: "Bad Gateway", status: 502 }, { status: 502 }),
      ),
    );
    act(() => result.current.fetchSource.run("folder", onResult));
    await waitFor(() => expect(result.current.fetchSource.error).not.toBeNull());
    expect(onResult).toHaveBeenCalledOnce();
  });
});
