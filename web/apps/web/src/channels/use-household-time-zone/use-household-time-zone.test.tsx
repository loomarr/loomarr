import { getChannelGuideMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { useHouseholdTimeZone } from "./use-household-time-zone";

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    {children}
  </QueryClientProvider>
);

describe("useHouseholdTimeZone", () => {
  it("is the zone the Guide tells times in", async () => {
    server.use(
      getChannelGuideMockHandler({ channels: [], fromMs: 0, toMs: 0, timezone: "Pacific/Auckland" }),
    );
    const { result } = renderHook(() => useHouseholdTimeZone(), { wrapper });
    await waitFor(() => expect(result.current).toBe("Pacific/Auckland"));
  });

  it("is undefined while the Guide can't be read, so times fall back to the device's zone", async () => {
    let asked = false;
    server.use(
      http.get("*/v1/guide", () => {
        asked = true;
        return HttpResponse.json({ title: "Unavailable" }, { status: 503 });
      }),
    );
    const { result } = renderHook(() => useHouseholdTimeZone(), { wrapper });
    await waitFor(() => expect(asked).toBe(true));
    expect(result.current).toBeUndefined();
  });
});
