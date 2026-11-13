import { getFillerWatchQueryOptions } from "@loomarr/api/endpoints/filler";
import { getSettingsListQueryOptions } from "@loomarr/api/endpoints/settings";
import type { QueryClient } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { meQueryOptions } from "@/auth/me-query";
import { warmDefectReport, warmFillerPage } from "./filler-loader";

// A client that answers `me` as `role` and records what was warmed, so a test can say which reads a
// role's page open asks for.
const clientFor = (role: "admin" | "member") => {
  const prefetched: unknown[][] = [];
  const queryClient = {
    ensureQueryData: vi.fn().mockResolvedValue({ status: 200, data: { role } }),
    prefetchQuery: vi.fn(async ({ queryKey }: { queryKey: unknown[] }) => {
      prefetched.push(queryKey);
    }),
  };
  return { queryClient: queryClient as unknown as QueryClient, prefetched, raw: queryClient };
};

describe("warmFillerPage", () => {
  it("warms the status line and the settings list for an admin", async () => {
    const { queryClient, prefetched } = clientFor("admin");
    await warmFillerPage(queryClient);
    expect(prefetched).toEqual([
      getFillerWatchQueryOptions().queryKey,
      getSettingsListQueryOptions().queryKey,
    ]);
  });

  it("leaves the admin-only settings list to a member", async () => {
    const { queryClient, prefetched } = clientFor("member");
    await warmFillerPage(queryClient);
    expect(prefetched).toEqual([getFillerWatchQueryOptions().queryKey]);
  });

  it("reads who is asking from the cache the authed layout primed", async () => {
    const { queryClient, raw } = clientFor("admin");
    await warmFillerPage(queryClient);
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
