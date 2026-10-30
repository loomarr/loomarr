import { getChannelSubtitlesMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { beforeEach, describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { useLiveSubtitles } from "./use-live-subtitles";

const english = { language: "eng", languageName: "English", title: "" };
const englishSDH = { language: "eng", languageName: "English", title: "SDH" };
const asked: URLSearchParams[] = [];

// A subtitled programme until 1,000,000, then a break with nothing to offer.
const stub = () =>
  server.use(
    getChannelSubtitlesMockHandler(({ request }) => {
      const params = new URL(request.url).searchParams;
      asked.push(params);
      return {
        airings: [
          {
            startMs: 0,
            stopMs: 1_000_000,
            tracks: [english, englishSDH],
            cues: params.get("language") ? [{ startMs: 990_000, endMs: 992_000, text: "Hello" }] : [],
          },
          { startMs: 1_000_000, stopMs: 1_120_000, tracks: [], cues: [] },
        ],
      };
    }),
  );

const wrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => createElement(QueryClientProvider, { client }, children);
};

describe("useLiveSubtitles", () => {
  beforeEach(() => {
    asked.length = 0;
    stub();
  });

  it("offers the programme at the playhead's tracks, labelled, asking only for tracks while off", async () => {
    const { result } = renderHook(() => useLiveSubtitles("ch-1", 995_000, "off", true), {
      wrapper: wrapper(),
    });
    await waitFor(() => expect(result.current.tracks).toEqual([english, englishSDH]));
    expect(result.current.tracks.map(result.current.labelOf)).toEqual(["English", "English · SDH"]);
    expect(result.current.active).toBe(-1);
    expect(result.current.cues).toBeUndefined();
    // The window steps by 30 s: from 15 s behind the playhead's step (990,000), 90 s long.
    expect([asked[0]?.get("from"), asked[0]?.get("to"), asked[0]?.has("language")]).toEqual([
      "975000",
      "1065000",
      false,
    ]);
  });

  it("asks for the chosen track's cues and hands them on, stable while the window's data is", async () => {
    const { result, rerender } = renderHook(({ at }) => useLiveSubtitles("ch-1", at, englishSDH, true), {
      initialProps: { at: 995_000 },
      wrapper: wrapper(),
    });
    await waitFor(() => expect(result.current.cues?.cues.map((c) => c.text)).toEqual(["Hello"]));
    expect(result.current.active).toBe(1);
    expect(result.current.cues).toMatchObject({ label: "English · SDH", language: "eng" });
    expect([asked.at(-1)?.get("language"), asked.at(-1)?.get("title")]).toEqual(["eng", "SDH"]);

    // A tick inside the same step reuses the window: no request, the same cues object.
    const cues = result.current.cues;
    const requests = asked.length;
    rerender({ at: 998_000 });
    expect(result.current.cues).toBe(cues);
    expect(asked.length).toBe(requests);
  });

  it("offers nothing once the playhead is in a break, however the viewer chose", async () => {
    const { result } = renderHook(() => useLiveSubtitles("ch-1", 1_005_000, english, true), {
      wrapper: wrapper(),
    });
    await waitFor(() => expect(asked.length).toBeGreaterThan(0));
    await waitFor(() => expect(result.current.tracks).toEqual([]));
    expect(result.current.active).toBe(-1);
    expect(result.current.cues).toBeUndefined();
  });

  it("asks nothing until the player is playing", () => {
    renderHook(() => useLiveSubtitles("ch-1", 995_000, english, false), { wrapper: wrapper() });
    expect(asked).toEqual([]);
  });
});
