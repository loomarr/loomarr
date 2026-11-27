import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { useOAuthRequest } from "./oauth-request";

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    {children}
  </QueryClientProvider>
);

describe("useOAuthRequest", () => {
  it("reads the waiting sign-in named by the continue path", async () => {
    let asked = "";
    server.use(
      http.get("*/v1/oauth/request", ({ request }) => {
        asked = new URL(request.url).searchParams.get("request") ?? "";
        return HttpResponse.json({ app: "Claude", sendsTo: "claude.ai", loopback: false });
      }),
    );
    const { result } = renderHook(
      () => useOAuthRequest("/v1/oauth/continue?request=Zb3k_Q9-xY2mN4pL7vR1sT6uW8aC0dEf"),
      { wrapper },
    );
    await waitFor(() =>
      expect(result.current.data).toEqual({ app: "Claude", sendsTo: "claude.ai", loopback: false }),
    );
    expect(asked).toBe("Zb3k_Q9-xY2mN4pL7vR1sT6uW8aC0dEf");
  });

  it("asks nothing when no sign-in is waiting", () => {
    let calls = 0;
    server.use(
      http.get("*/v1/oauth/request", () => {
        calls += 1;
        return HttpResponse.json({});
      }),
    );
    const { result } = renderHook(() => useOAuthRequest(undefined), { wrapper });
    expect(result.current.fetchStatus).toBe("idle");
    expect(calls).toBe(0);
  });
});
