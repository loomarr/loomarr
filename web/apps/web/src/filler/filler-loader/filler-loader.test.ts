import { getFillerWatchQueryOptions } from "@loomarr/api/endpoints/filler";
import { getSettingsListQueryOptions } from "@loomarr/api/endpoints/settings";
import type { QueryClient } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { meQueryOptions } from "@/auth/me-query";
import { openFillerPage, warmDefectReport } from "./filler-loader";

// A client that answers `me` as `role` and records what was warmed and what was ensured, so a test
// can say which reads a role's page open asks for and which of them it cannot open without.
const clientFor = (role: "admin" | "member") => {
  const prefetched: unknown[][] = [];
  const ensured: unknown[][] = [];
  const queryClient = {
    ensureQueryData: vi.fn(async ({ queryKey }: { queryKey: unknown[] }) => {
      if (queryKey[0] !== meQueryOptions().queryKey[0]) ensured.push(queryKey);
      return { status: 200, data: { role } };
    }),
    prefetchQuery: vi.fn(async ({ queryKey }: { queryKey: unknown[] }) => {
      prefetched.push(queryKey);
    }),
  };
  return { queryClient: queryClient as unknown as QueryClient, prefetched, ensured, raw: queryClient };
};

describe("openFillerPage", () => {
  it("ensures an admin's settings list and warms the status line", async () => {
    const { queryClient, prefetched, ensured } = clientFor("admin");
    await openFillerPage(queryClient);
    expect(ensured).toEqual([getSettingsListQueryOptions().queryKey]);
    expect(prefetched).toEqual([getFillerWatchQueryOptions().queryKey]);
  });

  it("ensures a member's watch summary and leaves the admin-only settings list", async () => {
    const { queryClient, prefetched, ensured } = clientFor("member");
    await openFillerPage(queryClient);
    expect(ensured).toEqual([getFillerWatchQueryOptions().queryKey]);
    expect(prefetched).toEqual([]);
  });

  it("reads who is asking from the cache the authed layout primed", async () => {
    const { queryClient, raw } = clientFor("admin");
    await openFillerPage(queryClient);
    expect(raw.ensureQueryData).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: meQueryOptions().queryKey }),
    );
  });
});

describe("warmDefectReport", () => {
  it("warms the first unlabelled page for an admin", async () => {
    const { queryClient, prefetched } = clientFor("admin");
    await warmDefectReport(queryClient);
    expect(prefetched).toHaveLength(1);
    expect(JSON.stringify(prefetched[0])).toContain("unlabelled");
  });

  it("asks nothing of a member, whose request the server would refuse", async () => {
    const { queryClient, prefetched } = clientFor("member");
    await warmDefectReport(queryClient);
    expect(prefetched).toEqual([]);
  });
});
