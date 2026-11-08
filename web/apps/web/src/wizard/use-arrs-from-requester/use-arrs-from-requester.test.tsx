import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { PropsWithChildren } from "react";
import { describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { useArrsFromRequester } from "./use-arrs-from-requester";

const wrapper = ({ children }: PropsWithChildren) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    {children}
  </QueryClientProvider>
);

// The wiring: Seerr is asked once while an arr is missing, never when both are set, and a fill
// tells the step so it can look for the folders the arr names.
describe("useArrsFromRequester", () => {
  it("asks Seerr once while an arr is missing", async () => {
    let asks = 0;
    server.use(
      http.post("*/v1/setup/arrs-from-requester", () => {
        asks++;
        return HttpResponse.json({
          arrs: [{ app: "sonarr", url: "http://sonarr:8989", filled: true, reachable: true }],
        });
      }),
    );
    const onFilled = vi.fn();
    const { result, rerender } = renderHook(() => useArrsFromRequester({ missing: true, onFilled }), {
      wrapper,
    });

    await waitFor(() => expect(result.current.filled).toHaveLength(1));
    rerender();
    expect(asks).toBe(1);
    expect(onFilled).toHaveBeenCalledTimes(1);
  });

  it("leaves Seerr alone when both are set", async () => {
    let asks = 0;
    server.use(
      http.post("*/v1/setup/arrs-from-requester", () => {
        asks++;
        return HttpResponse.json({ arrs: [] });
      }),
    );
    renderHook(() => useArrsFromRequester({ missing: false }), { wrapper });
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(asks).toBe(0);
  });
});
