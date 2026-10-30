// @vitest-environment jsdom

import type { ChannelSubtitlesOutputBody } from "@loomarr/api/models/channelSubtitlesOutputBody";
import type { ChannelSubtitlesSource } from "@loomarr/core/channel-subtitles";
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

import type { LiveSubtitles } from "../src/live-subtitles";
import { type PairedSubtitles, usePairedSubtitles } from "../src/paired-subtitles";

const english = { language: "eng", languageName: "English", title: "" };
const windowWith = (text: string): ChannelSubtitlesOutputBody => ({
  airings: [{ cues: [{ endMs: 60_000, startMs: 0, text }], startMs: 0, stopMs: 600_000, tracks: [english] }],
});

interface ProbeProps {
  at: number;
  choice: LiveSubtitles;
  id: string | undefined;
}

// Renders the hook at a playhead and reports each value it returns.
const mount = async (source: ChannelSubtitlesSource, props: ProbeProps) => {
  const seen: PairedSubtitles[] = [];
  const Probe = ({ at, choice, id }: ProbeProps) => {
    seen.push(usePairedSubtitles(source, id, at, choice));
    return null;
  };
  const root = createRoot(document.createElement("div"));
  await act(async () => root.render(createElement(Probe, props)));
  return {
    current: () => seen.at(-1),
    move: (next: ProbeProps) => act(async () => root.render(createElement(Probe, next))),
    unmount: () => act(async () => root.unmount()),
  };
};

describe("the subtitles at the playhead (#2127)", () => {
  it("lists the programme's tracks while off, and its cues once one is chosen", async () => {
    const load = vi.fn().mockResolvedValue(windowWith("Hello"));
    const probe = await mount({ load }, { at: 10_000, choice: "off", id: "a" });
    expect(probe.current()).toMatchObject({ active: -1, cues: [], tracks: [english] });

    await probe.move({ at: 10_000, choice: english, id: "a" });
    expect(load).toHaveBeenLastCalledWith(
      "a",
      { from: -15_000, language: "eng", title: "", to: 75_000 },
      expect.any(AbortSignal),
    );
    expect(probe.current()?.cues.map((cue) => cue.text)).toEqual(["Hello"]);
    await probe.unmount();
  });

  it("asks again only as the playhead steps, keeping the last window meanwhile", async () => {
    let answerNext: (body: ChannelSubtitlesOutputBody) => void = () => undefined;
    const load = vi
      .fn()
      .mockResolvedValueOnce(windowWith("First"))
      .mockReturnValueOnce(new Promise((r) => (answerNext = r)));
    const probe = await mount({ load }, { at: 1_000, choice: english, id: "a" });
    await probe.move({ at: 29_000, choice: english, id: "a" });
    expect(load).toHaveBeenCalledOnce();

    await probe.move({ at: 31_000, choice: english, id: "a" });
    expect(load).toHaveBeenCalledTimes(2);
    expect(probe.current()?.cues.map((cue) => cue.text)).toEqual(["First"]);
    await act(async () => answerNext(windowWith("Second")));
    expect(probe.current()?.cues.map((cue) => cue.text)).toEqual(["Second"]);
    await probe.unmount();
  });

  it("drops another channel's subtitles at once", async () => {
    const load = vi
      .fn()
      .mockResolvedValueOnce(windowWith("A"))
      .mockReturnValueOnce(new Promise(() => undefined));
    const probe = await mount({ load }, { at: 1_000, choice: english, id: "a" });
    await probe.move({ at: 1_000, choice: english, id: "b" });
    expect(probe.current()).toMatchObject({ cues: [], tracks: [] });
    await probe.unmount();
  });
});
