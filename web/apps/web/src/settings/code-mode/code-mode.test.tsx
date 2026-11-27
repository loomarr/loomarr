import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { useCodeModeDevices, useSetCodeMode } from "./code-mode";

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    {children}
  </QueryClientProvider>
);

describe("code mode hooks", () => {
  it("reads which devices have Code Mode on", async () => {
    server.use(http.get("*/v1/agents/code-mode", () => HttpResponse.json({ devices: ["a", "b"] })));
    const { result } = renderHook(() => useCodeModeDevices(true), { wrapper });
    await waitFor(() => expect(result.current.data).toEqual(new Set(["a", "b"])));
  });

  it("asks nothing when not enabled", () => {
    const { result } = renderHook(() => useCodeModeDevices(false), { wrapper });
    expect(result.current.fetchStatus).toBe("idle");
  });

  it("sets one device, with the CSRF header a cookie session needs", async () => {
    let seen: { id: string; on: unknown; csrf: string | null } | undefined;
    server.use(
      http.put("*/v1/agents/code-mode/:id", async ({ params, request }) => {
        seen = {
          id: String(params.id),
          on: ((await request.json()) as { on: unknown }).on,
          csrf: request.headers.get("X-Loomarr-Csrf"),
        };
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { result } = renderHook(() => useSetCodeMode(), { wrapper });
    result.current.mutate({ id: "dev/1", on: false });
    await waitFor(() => expect(seen).toEqual({ id: "dev/1", on: false, csrf: "1" }));
  });
});
