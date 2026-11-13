import { getMeMockHandler } from "@loomarr/api/msw";
import { customFetch } from "@loomarr/api/mutator";
import { QueryClient } from "@tanstack/react-query";
import { waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meQueryOptions } from "@/auth/me-query";
import { server } from "@/test/msw/server";
import { watchSessionExpiry } from "./session-expiry";

const member = {
  local: true,
  offlineLogin: false,
  id: "u1",
  name: "Ada",
  role: "member",
  autoApprove: false,
  disabled: false,
  quota: 0,
} as const;

// The spec declares errors only as `default:`, so no generated handler answers 401 (see use-auth.test).
const refuse = (method: "get" | "post", path: string) =>
  http[method](`*${path}`, () => HttpResponse.json({ title: "Not signed in" }, { status: 401 }));

const stops: (() => void)[] = [];
afterEach(() => {
  for (const stop of stops.splice(0)) stop();
});

const signedIn = async () => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  server.use(getMeMockHandler(member));
  await queryClient.fetchQuery(meQueryOptions());
  const onSignedOut = vi.fn();
  stops.push(watchSessionExpiry(queryClient, onSignedOut));
  return { onSignedOut, queryClient };
};

const failingRead = (queryClient: QueryClient) =>
  queryClient
    .fetchQuery({ queryKey: ["/v1/channels"], queryFn: () => customFetch("/v1/channels") })
    .catch(() => undefined);

describe("watchSessionExpiry", () => {
  it("signs a revoked session out when a read answers 401 and so does `me`", async () => {
    const { onSignedOut, queryClient } = await signedIn();
    server.use(refuse("get", "/v1/channels"), refuse("get", "/v1/auth/me"));

    await failingRead(queryClient);

    await waitFor(() => expect(onSignedOut).toHaveBeenCalledTimes(1));
    expect(queryClient.getQueryData(meQueryOptions().queryKey)).toBeUndefined();
  });

  it("keeps the session when a 401 is the answer to the request itself, not the session", async () => {
    const { onSignedOut, queryClient } = await signedIn();
    // A wrong current password is a 401 while the cookie is still good.
    server.use(refuse("post", "/v1/auth/password"));
    const me = vi.fn();
    server.events.on("request:start", ({ request }) => {
      if (new URL(request.url).pathname === "/v1/auth/me") me();
    });

    await queryClient
      .getMutationCache()
      .build(queryClient, { mutationFn: () => customFetch("/v1/auth/password", { method: "POST" }) })
      .execute(undefined)
      .catch(() => undefined);

    await waitFor(() => expect(me).toHaveBeenCalled());
    await new Promise((settle) => setTimeout(settle, 20));
    expect(onSignedOut).not.toHaveBeenCalled();
    expect(queryClient.getQueryData(meQueryOptions().queryKey)).toMatchObject({ status: 200 });
    server.events.removeAllListeners();
  });

  it("does nothing for a viewer who was never signed in", async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const onSignedOut = vi.fn();
    stops.push(watchSessionExpiry(queryClient, onSignedOut));
    server.use(refuse("get", "/v1/channels"), refuse("get", "/v1/auth/me"));

    await failingRead(queryClient);
    await new Promise((settle) => setTimeout(settle, 20));

    expect(onSignedOut).not.toHaveBeenCalled();
  });
});
