import { channelsApi } from "@loomarr/api";
import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { delay, HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { toast } from "sonner";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { channel } from "@/test/fixtures/channels";
import { server } from "@/test/msw/server";
import { useChannelPolicySave } from "./use-channel-policy-save";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

type Stored = { revision: number; policy: ChannelPolicy };
type Patch = { revision: number; policy: ChannelPolicy };

// A channel endpoint with the backend's revision contract: a stale revision is a 409 that
// changes nothing; an accepted PATCH replaces the policy wholesale and bumps the revision.
const stubChannel = (stored: Stored, failWith?: number) => {
  const patches: Patch[] = [];
  const current = () => channel({ id: "ch-1", revision: stored.revision, policy: stored.policy });
  server.use(
    http.get("*/v1/channels/:id", () => HttpResponse.json(current())),
    http.patch("*/v1/channels/:id", async ({ request }) => {
      const body = (await request.json()) as Patch;
      patches.push(body);
      await delay(30);
      if (failWith) return HttpResponse.json({ title: "Couldn't save the channel" }, { status: failWith });
      if (body.revision !== stored.revision)
        return HttpResponse.json({ title: "Channel changed" }, { status: 409 });
      stored.policy = body.policy;
      stored.revision += 1;
      return HttpResponse.json(current());
    }),
  );
  return { patches };
};

// The layout has already read the channel when an editor saves, so the cache starts warm.
const renderSave = (stored: Stored) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(channelsApi.getGetChannelQueryKey("ch-1"), {
    status: 200,
    data: channel({ id: "ch-1", revision: stored.revision, policy: stored.policy }),
    headers: new Headers(),
  });
  const onSaved = vi.fn();
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const hook = renderHook(({ id }) => useChannelPolicySave(id, onSaved), {
    wrapper,
    initialProps: { id: "ch-1" },
  });
  return { ...hook, onSaved };
};

describe("useChannelPolicySave", () => {
  beforeEach(() => vi.clearAllMocks());

  it("has nothing pending before an edit", () => {
    stubChannel({ revision: 1, policy: {} });
    const { result } = renderSave({ revision: 1, policy: {} });
    expect(result.current.pending).toBeUndefined();
    expect(result.current.isSaving).toBe(false);
  });

  it("sends one PATCH at a time and coalesces the edits made meanwhile into the next", async () => {
    const stored: Stored = { revision: 1, policy: {} };
    const { patches } = stubChannel(stored);
    const { result, onSaved } = renderSave(stored);

    act(() => result.current.save({ ordering: "sequential" }));
    // Shown at once, so the next edit is built on this one rather than the saved policy.
    expect(result.current.pending).toEqual({ ordering: "sequential" });
    act(() => result.current.save({ ordering: "sequential", midRoll: true }));
    act(() => result.current.save({ ordering: "sequential", midRoll: true, breaksPerHour: 2 }));

    await waitFor(() => expect(result.current.isSaving).toBe(false));
    expect(patches).toEqual([
      { revision: 1, policy: { ordering: "sequential" } },
      // The second PATCH carries the revision the first one returned, and every later edit.
      { revision: 2, policy: { ordering: "sequential", midRoll: true, breaksPerHour: 2 } },
    ]);
    expect(stored.policy).toEqual({ ordering: "sequential", midRoll: true, breaksPerHour: 2 });
    expect(result.current.pending).toBeUndefined();
    expect(onSaved).toHaveBeenCalledTimes(1);
    expect(toast.success).toHaveBeenCalledTimes(1);
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("on a 409 refetches and re-applies only the operator's edits, keeping the other writer's", async () => {
    // The page read revision 1 with auto-curate on; another writer has since saved revision 2.
    const stored: Stored = { revision: 2, policy: { autoCurate: {}, ordering: "shuffle" } };
    const { patches } = stubChannel(stored);
    const { result } = renderSave({ revision: 1, policy: { autoCurate: {} } });

    // The operator switches auto-curate off: the opt-out is the key's absence.
    act(() => result.current.save({ breaksPerHour: 2 }));

    await waitFor(() => expect(result.current.isSaving).toBe(false));
    expect(patches.map((p) => p.revision)).toEqual([1, 2]);
    expect(stored.policy).toEqual({ ordering: "shuffle", breaksPerHour: 2 });
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("gives up and says so when the channel keeps changing underneath", async () => {
    const stored: Stored = { revision: 1, policy: {} };
    const patches: Patch[] = [];
    server.use(
      // Every read is already stale by the time the PATCH lands.
      http.get("*/v1/channels/:id", () => {
        stored.revision += 1;
        return HttpResponse.json(channel({ id: "ch-1", revision: stored.revision - 1, policy: {} }));
      }),
      http.patch("*/v1/channels/:id", async ({ request }) => {
        patches.push((await request.json()) as Patch);
        return HttpResponse.json({ title: "Channel changed" }, { status: 409 });
      }),
    );
    const { result, onSaved } = renderSave(stored);

    act(() => result.current.save({ ordering: "sequential" }));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Channel changed"));
    expect(patches).toHaveLength(4);
    expect(result.current.pending).toBeUndefined();
    expect(onSaved).toHaveBeenCalled();
  });

  it("keeps one channel's unsaved policy off another channel the layout surfs to", async () => {
    const stored: Stored = { revision: 1, policy: {} };
    const { patches } = stubChannel(stored);
    const { result, rerender } = renderSave(stored);

    act(() => result.current.save({ ordering: "sequential" }));
    rerender({ id: "ch-2" });
    expect(result.current.pending).toBeUndefined();
    expect(result.current.isSaving).toBe(false);

    rerender({ id: "ch-1" });
    await waitFor(() => expect(stored.policy).toEqual({ ordering: "sequential" }));
    expect(patches).toHaveLength(1);
  });

  it("shows the error and drops back to the saved policy when a save genuinely fails", async () => {
    const stored: Stored = { revision: 1, policy: {} };
    const { patches } = stubChannel(stored, 500);
    const { result, onSaved } = renderSave(stored);

    act(() => result.current.save({ ordering: "sequential" }));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Couldn't save the channel"));
    expect(patches).toHaveLength(1);
    expect(result.current.pending).toBeUndefined();
    expect(onSaved).toHaveBeenCalled();
    expect(toast.success).not.toHaveBeenCalled();
  });
});
