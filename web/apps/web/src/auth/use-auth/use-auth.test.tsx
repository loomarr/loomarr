import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useAuth } from "./use-auth";

const makeWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const stubFetch = (status: number, body: unknown) =>
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve(
        new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } }),
      ),
    ),
  );

afterEach(() => vi.restoreAllMocks());

describe("useAuth", () => {
  it("exposes the signed-in admin identity", async () => {
    stubFetch(200, { id: "u1", name: "Ada", role: "admin", autoApprove: true, disabled: false, quota: 0 });
    const { result } = renderHook(() => useAuth(), { wrapper: makeWrapper() });

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.isAuthenticated).toBe(true);
    expect(result.current.isAdmin).toBe(true);
    expect(result.current.user?.name).toBe("Ada");
  });

  it("is unauthenticated when me returns 401", async () => {
    stubFetch(401, { title: "Unauthorized" });
    const { result } = renderHook(() => useAuth(), { wrapper: makeWrapper() });

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.isAuthenticated).toBe(false);
    expect(result.current.isAdmin).toBe(false);
    expect(result.current.user).toBeUndefined();
  });

  it("flags a member as authenticated but not admin", async () => {
    stubFetch(200, { id: "u2", name: "Bo", role: "member", autoApprove: false, disabled: false, quota: 5 });
    const { result } = renderHook(() => useAuth(), { wrapper: makeWrapper() });

    await waitFor(() => expect(result.current.isAuthenticated).toBe(true));
    expect(result.current.isAdmin).toBe(false);
  });
});
