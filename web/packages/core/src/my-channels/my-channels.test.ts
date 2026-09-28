import type { MyChannelsOutputBody } from "@loomarr/api/models/myChannelsOutputBody";
import { describe, expect, it, vi } from "vitest";

import { createMyChannelsController, createMyChannelsPort } from "./my-channels";
import type { MyChannelsPort } from "./my-channels.type";

const body = (favourites: string[], recent: string[]): MyChannelsOutputBody => ({
  favourites: favourites.map((channelId) => ({ addedAt: "2026-09-28T20:00:00Z", channelId })),
  recent: recent.map((channelId) => ({ channelId, tunedAt: "2026-09-28T20:00:00Z" })),
});

const port = (overrides: Partial<MyChannelsPort> = {}): MyChannelsPort => ({
  load: vi.fn(async () => body(["seven"], ["nine", "seven"])),
  recordTune: vi.fn(async () => body(["seven"], ["four", "nine", "seven"])),
  setFavourite: vi.fn(async () => body(["seven", "nine"], ["nine", "seven"])),
  ...overrides,
});

describe("my channels", () => {
  it("reads both lists in the server's order", async () => {
    const controller = createMyChannelsController({ port: port() });
    expect(controller.getSnapshot().status).toBe("loading");
    await controller.refresh();
    expect(controller.getSnapshot()).toEqual({
      favouriteIds: ["seven"],
      recentIds: ["nine", "seven"],
      status: "ready",
    });
  });

  it("moves a settled tune to the front at once, then takes the server's lists", async () => {
    let settle: (value: MyChannelsOutputBody) => void = () => undefined;
    const recordTune = vi.fn(() => new Promise<MyChannelsOutputBody>((resolve) => (settle = resolve)));
    const controller = createMyChannelsController({ port: port({ recordTune }) });
    await controller.refresh();

    const pending = controller.recordTune("seven");
    expect(controller.getSnapshot().recentIds).toEqual(["seven", "nine"]);
    settle(body(["seven"], ["seven", "nine", "two"]));
    await pending;
    expect(controller.getSnapshot().recentIds).toEqual(["seven", "nine", "two"]);
  });

  it("keeps a refused star from sticking", async () => {
    const setFavourite = vi.fn(async () => {
      throw new Error("403");
    });
    const controller = createMyChannelsController({ port: port({ setFavourite }) });
    await controller.refresh();
    const seen: string[][] = [];
    controller.subscribe(() => seen.push([...controller.getSnapshot().favouriteIds]));

    await controller.setFavourite("nine", true);
    expect(seen).toEqual([["seven", "nine"], ["seven"]]);
    expect(controller.getSnapshot().favouriteIds).toEqual(["seven"]);
  });

  it("reports a failed read without inventing lists", async () => {
    const controller = createMyChannelsController({
      port: port({ load: vi.fn(async () => Promise.reject(new Error("offline"))) }),
    });
    await controller.refresh();
    expect(controller.getSnapshot()).toEqual({ favouriteIds: [], recentIds: [], status: "error" });
  });

  it("speaks the #1666 routes: GET the lists, PUT a tune, PUT and DELETE a star", async () => {
    const request = vi.fn(
      async (_url: string, _init: RequestInit) => new Response(JSON.stringify(body([], [])), { status: 200 }),
    );
    const http = createMyChannelsPort(request as unknown as typeof fetch);
    await http.load(new AbortController().signal);
    await http.recordTune("seven");
    await http.setFavourite("seven", true);
    await http.setFavourite("seven", false);
    expect(request.mock.calls.map(([url, init]) => `${init.method} ${url}`)).toEqual([
      "GET /v1/me/channels",
      "PUT /v1/me/recent-channels/seven",
      "PUT /v1/me/favourites/seven",
      "DELETE /v1/me/favourites/seven",
    ]);
  });
});
