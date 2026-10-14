import { channelsApi, type LineupEntryDTO } from "@loomarr/api";
import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import type { UpdateChannelInputBody } from "@loomarr/api/models/updateChannelInputBody";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { delay, HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { toast } from "sonner";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { channel } from "@/test/fixtures/channels";
import { server } from "@/test/msw/server";
import { ignoreFailure, useChannelSave } from "./use-channel-save";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

type Stored = { revision: number; name?: string; policy: ChannelPolicy; lineup?: LineupEntryDTO[] };
type Patch = Omit<UpdateChannelInputBody, "$schema">;

// A channel endpoint with the backend's revision contract: a stale revision is a 409 "Channel
// changed" that changes nothing; an accepted PATCH sets the fields it carries (policy and lineup
// wholesale) and bumps the revision.
const stubChannel = (stored: Stored, failWith?: number | ((body: Patch) => Response | undefined)) => {
  const patches: Patch[] = [];
  const current = () => channel({ id: "ch-1", ...stored, lineup: stored.lineup ?? [] });
  server.use(
    http.get("*/v1/channels/:id", () => HttpResponse.json(current())),
    http.patch("*/v1/channels/:id", async ({ request }) => {
      const body = (await request.json()) as Patch;
      patches.push(body);
      await delay(30);
      if (typeof failWith === "function") {
        const failure = failWith(body);
        if (failure) return failure;
      } else if (failWith) {
        return HttpResponse.json({ title: "Couldn't save the channel" }, { status: failWith });
      }
      if (body.revision !== stored.revision)
        return HttpResponse.json({ title: "Channel changed" }, { status: 409 });
      if (body.policy) stored.policy = body.policy;
      if (body.lineup) stored.lineup = body.lineup;
      if (body.name !== undefined) stored.name = body.name;
      stored.revision += 1;
      return HttpResponse.json(current());
    }),
  );
  return { patches };
};

// The layout has already read the channel when an editor saves, so the cache starts warm unless a
// test says otherwise (the guide's row menu has no channel read).
const renderSave = (stored: Stored | undefined) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  if (stored) {
    client.setQueryData(channelsApi.getGetChannelQueryKey("ch-1"), {
      status: 200,
      data: channel({ id: "ch-1", ...stored, lineup: stored.lineup ?? [] }),
      headers: new Headers(),
    });
  }
  const invalidate = vi.spyOn(client, "invalidateQueries");
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const hook = renderHook(({ id }) => useChannelSave(id), { wrapper, initialProps: { id: "ch-1" } });
  // A second editor on the same page, e.g. the lineup editor beside the policy fields.
  const other = renderHook(() => useChannelSave("ch-1"), { wrapper });
  return { ...hook, other, invalidate };
};

const velvet: LineupEntryDTO = { key: "movie:tmdb:90000949", name: "The Velvet Cartographer", year: 1995 };

describe("useChannelSave", () => {
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
    const { result } = renderSave(stored);

    act(() => void result.current.save({ policy: { ordering: "sequential" } }));
    // Shown at once, so the next edit is built on this one rather than the saved policy.
    expect(result.current.pending).toEqual({ policy: { ordering: "sequential" } });
    act(() => void result.current.save({ policy: { ordering: "sequential", midRoll: true } }));
    act(
      () => void result.current.save({ policy: { ordering: "sequential", midRoll: true, breaksPerHour: 2 } }),
    );

    await waitFor(() => expect(result.current.isSaving).toBe(false));
    expect(patches).toEqual([
      { revision: 1, policy: { ordering: "sequential" } },
      // The second PATCH carries the revision the first one returned, and every later edit.
      { revision: 2, policy: { ordering: "sequential", midRoll: true, breaksPerHour: 2 } },
    ]);
    expect(stored.policy).toEqual({ ordering: "sequential", midRoll: true, breaksPerHour: 2 });
    expect(result.current.pending).toBeUndefined();
    expect(toast.success).toHaveBeenCalledTimes(1);
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("shares one queue between every editor of a channel, and sends only what is still unsaved", async () => {
    const stored: Stored = { revision: 1, policy: {} };
    const { patches } = stubChannel(stored);
    const { result, other } = renderSave(stored);

    let first: Promise<void> | undefined;
    let second: Promise<void> | undefined;
    act(() => {
      first = result.current.save({ policy: { ordering: "sequential" } });
    });
    // The other editor sees the pending edit and queues behind it.
    expect(other.result.current.isSaving).toBe(true);
    act(() => {
      second = other.result.current.save({ lineup: [velvet] });
    });
    await act(async () => {
      await Promise.all([first, second]);
    });

    expect(patches).toEqual([
      { revision: 1, policy: { ordering: "sequential" } },
      // The policy already landed, so the follow-up carries only the lineup.
      { revision: 2, lineup: [velvet] },
    ]);
    expect(stored).toMatchObject({ revision: 3, policy: { ordering: "sequential" }, lineup: [velvet] });
  });

  it("on a 409 refetches and re-applies only the operator's edits, keeping the other writer's", async () => {
    // The page read revision 1 with auto-curate on; another writer has since saved revision 2.
    const stored: Stored = { revision: 2, policy: { autoCurate: {}, ordering: "shuffle" } };
    const { patches } = stubChannel(stored);
    const { result } = renderSave({ revision: 1, policy: { autoCurate: {} } });

    // The operator switches auto-curate off (the opt-out is the key's absence) and renames.
    act(() => void result.current.save({ policy: { breaksPerHour: 2 } }));
    act(() => void result.current.save({ name: "Midnight Noir" }));

    await waitFor(() => expect(result.current.isSaving).toBe(false));
    expect(patches.map((p) => p.revision)).toEqual([1, 2]);
    expect(stored.policy).toEqual({ ordering: "shuffle", breaksPerHour: 2 });
    expect(stored.name).toBe("Midnight Noir");
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("reads the channel first when nothing has cached it", async () => {
    const stored: Stored = { revision: 7, policy: {} };
    const { patches } = stubChannel(stored);
    const { result } = renderSave(undefined);

    await act(() => result.current.save({ status: "paused" }, { success: "Channel paused" }));

    expect(patches).toEqual([{ revision: 7, status: "paused" }]);
    expect(toast.success).toHaveBeenCalledWith("Channel paused");
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
    const { result } = renderSave(stored);

    act(() => void result.current.save({ policy: { ordering: "sequential" } }).catch(ignoreFailure));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Channel changed"));
    expect(patches).toHaveLength(4);
    expect(result.current.pending).toBeUndefined();
  });

  it("does not retry a 409 that is not a stale revision", async () => {
    const patches: Patch[] = [];
    server.use(
      http.patch("*/v1/channels/:id", async ({ request }) => {
        patches.push((await request.json()) as Patch);
        return HttpResponse.json({ title: "Channel already exists" }, { status: 409 });
      }),
    );
    const { result } = renderSave({ revision: 1, policy: {} });

    let saving: Promise<void> | undefined;
    act(() => {
      saving = result.current.save({ number: 2 });
    });

    await expect(saving).rejects.toMatchObject({ status: 409 });
    expect(patches).toHaveLength(1);
    expect(toast.error).toHaveBeenCalledWith("Channel already exists");
  });

  it("keeps one channel's unsaved edits off another channel the layout surfs to", async () => {
    const stored: Stored = { revision: 1, policy: {} };
    const { patches } = stubChannel(stored);
    const { result, rerender } = renderSave(stored);

    act(() => void result.current.save({ policy: { ordering: "sequential" } }));
    rerender({ id: "ch-2" });
    expect(result.current.pending).toBeUndefined();
    expect(result.current.isSaving).toBe(false);

    rerender({ id: "ch-1" });
    await waitFor(() => expect(stored.policy).toEqual({ ordering: "sequential" }));
    expect(patches).toHaveLength(1);
  });

  it("rejects, shows the error and drops back to what is saved when a save genuinely fails", async () => {
    const stored: Stored = { revision: 1, policy: {} };
    const { patches } = stubChannel(stored, 500);
    const { result, invalidate } = renderSave(stored);

    let saving: Promise<void> | undefined;
    act(() => {
      saving = result.current.save({ lineup: [velvet] });
    });
    expect(result.current.pending).toEqual({ lineup: [velvet] });

    await expect(saving).rejects.toMatchObject({ status: 500 });
    expect(toast.error).toHaveBeenCalledWith("Couldn't save the channel");
    expect(patches).toHaveLength(1);
    expect(result.current.pending).toBeUndefined();
    // The page refetches, so it shows what was actually saved.
    expect(invalidate).toHaveBeenCalledWith({ queryKey: channelsApi.getGetChannelQueryKey("ch-1") });
    expect(toast.success).not.toHaveBeenCalled();
  });

  it("fails only the edit the server refused, and still saves the edits coalesced with it", async () => {
    const stored: Stored = { revision: 1, policy: {} };
    const taken = () => HttpResponse.json({ title: "Channel already exists" }, { status: 409 });
    const { patches } = stubChannel(stored, (body) => (body.number === 2 ? taken() : undefined));
    const { result } = renderSave(stored);

    // A policy save goes out; a lineup edit and a renumber onto a taken number queue behind it.
    let lineup: Promise<void> | undefined;
    let number: Promise<void> | undefined;
    act(() => void result.current.save({ policy: { ordering: "sequential" } }));
    act(() => {
      lineup = result.current.save({ lineup: [velvet] });
      number = result.current.save({ number: 2 });
    });

    await expect(number).rejects.toMatchObject({ status: 409 });
    await expect(lineup).resolves.toBeUndefined();
    expect(stored).toMatchObject({ policy: { ordering: "sequential" }, lineup: [velvet] });
    // The coalesced PATCH was refused, so each field went out alone to find the one at fault.
    expect(patches).toEqual([
      { revision: 1, policy: { ordering: "sequential" } },
      { revision: 2, lineup: [velvet], number: 2 },
      { revision: 2, lineup: [velvet] },
      { revision: 3, number: 2 },
    ]);
    expect(toast.error).toHaveBeenCalledWith("Channel already exists");
    await waitFor(() => expect(result.current.pending).toBeUndefined());
  });

  it("keeps an earlier round saved when a later round fails", async () => {
    const stored: Stored = { revision: 1, policy: {} };
    const refused = () => HttpResponse.json({ title: "Invalid channel" }, { status: 422 });
    stubChannel(stored, (body) => (body.name === "" ? refused() : undefined));
    const { result } = renderSave(stored);

    let first: Promise<void> | undefined;
    let second: Promise<void> | undefined;
    act(() => {
      first = result.current.save({ policy: { ordering: "sequential" } });
    });
    act(() => {
      second = result.current.save({ name: "" });
    });

    await expect(first).resolves.toBeUndefined();
    await expect(second).rejects.toMatchObject({ status: 422 });
    expect(stored.policy).toEqual({ ordering: "sequential" });
    expect(toast.error).toHaveBeenCalledWith("Invalid channel");
  });

  it("on a 409 rebases a lineup edit onto the other writer's lineup", async () => {
    const choir: LineupEntryDTO = { key: "movie:tmdb:90009426", name: "The Seventh Satellite Choir", year: 1991 };
    const marigold: LineupEntryDTO = { key: "movie:tmdb:90000106", name: "Marigold Protocol", year: 1987 };
    // The page read [Velvet, Choir]; another writer has since added Marigold.
    const stored: Stored = { revision: 2, policy: {}, lineup: [velvet, choir, marigold] };
    stubChannel(stored);
    const { result } = renderSave({ revision: 1, policy: {}, lineup: [velvet, choir] });

    // The operator removes Choir.
    await act(() => result.current.save({ lineup: [velvet] }));

    expect(stored.lineup).toEqual([velvet, marigold]);
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("keeps a named success message when a plain edit coalesces after it", async () => {
    const stored: Stored = { revision: 1, policy: {} };
    stubChannel(stored);
    const { result } = renderSave(stored);

    act(() => void result.current.save({ policy: { ordering: "sequential" } }, { success: "Rules updated" }));
    await act(() => result.current.save({ name: "Midnight Noir" }));

    expect(toast.success).toHaveBeenCalledTimes(1);
    expect(toast.success).toHaveBeenCalledWith("Rules updated");
  });
});
