import { getMeMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { useAuth } from "./use-auth";

const makeWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

// ⚠ The 401 case is hand-written where the success cases are generated, and the reason is a
// property of the SPEC rather than a shortcut: this API declares errors with OpenAPI's `default:`
// (RFC 7807 problem+json) on 132 of its 134 operations rather than enumerating 4xx codes — there
// are ZERO explicit 4xx/5xx responses in the document. `default` carries no status code, so
// there is nothing for orval to generate an error handler from; its handlers hardcode the success
// status.
//
// ⚠ Hand-writing this path is still safe against a route rename, and that is worth understanding
// rather than taking on trust. If `/v1/auth/me` moved, this stale handler would simply never
// match — but the hook would request the NEW path, nothing would handle it, and the
// unhandled-request guard in `src/test/msw/server.ts` fails the test by name. The failure mode is
// "no handler" (loud), not "wrong data" (silent).
const unauthorized = () =>
  http.get("*/v1/auth/me", () => HttpResponse.json({ title: "Unauthorized" }, { status: 401 }));

describe("useAuth", () => {
  it("exposes the signed-in admin identity", async () => {
    server.use(
      getMeMockHandler({
        local: true,
        id: "u1",
        name: "Ada",
        role: "admin",
        autoApprove: true,
        disabled: false,
        quota: 0,
      }),
    );
    const { result } = renderHook(() => useAuth(), { wrapper: makeWrapper() });

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.isAuthenticated).toBe(true);
    expect(result.current.isAdmin).toBe(true);
    expect(result.current.user?.name).toBe("Ada");
  });

  it("is unauthenticated when me returns 401", async () => {
    server.use(unauthorized());
    const { result } = renderHook(() => useAuth(), { wrapper: makeWrapper() });

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.isAuthenticated).toBe(false);
    expect(result.current.isAdmin).toBe(false);
    expect(result.current.user).toBeUndefined();
  });

  it("flags a member as authenticated but not admin", async () => {
    server.use(
      getMeMockHandler({
        local: true,
        id: "u2",
        name: "Bo",
        role: "member",
        autoApprove: false,
        disabled: false,
        quota: 5,
      }),
    );
    const { result } = renderHook(() => useAuth(), { wrapper: makeWrapper() });

    await waitFor(() => expect(result.current.isAuthenticated).toBe(true));
    expect(result.current.isAdmin).toBe(false);
  });
});
