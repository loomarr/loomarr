// @vitest-environment jsdom

import { createGuideController } from "@loomarr/core/guide";
import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";

import { GuideJourney } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// This package has no DOM lib (it targets React Native too), so the slice of the jsdom document
// the test touches is typed here.
type Node = { click(): void; textContent: string | null };
type Doc = {
  body: {
    append(node: unknown): void;
    replaceChildren(): void;
    textContent: string | null;
    querySelector(s: string): Node | null;
    querySelectorAll(s: string): ArrayLike<Node>;
  };
  createElement(tag: string): Parameters<typeof createRoot>[0];
};
const doc = (globalThis as unknown as { document: Doc }).document;

const sourceGuide = {
  channels: [
    {
      airings: [
        {
          kind: "program" as const,
          scheduleBlockId: "teapot",
          startMs: 0,
          stopMs: 1_800_000,
          title: "The Borrowed Teapot",
        },
      ],
      channelId: "cartoons",
      name: "Cartoon Classics",
      number: 1,
      pendingCount: 0,
      status: "live" as const,
    },
  ],
  fromMs: 0,
  timezone: "UTC",
  toMs: 3_600_000,
};

afterEach(() => {
  doc.body.replaceChildren();
});

const markup = (controller: ReturnType<typeof createGuideController>) =>
  renderToStaticMarkup(
    <LoomarrProvider>
      <GuideJourney controller={controller} onTune={vi.fn()} preferredChannelId="cartoons" />
    </LoomarrProvider>,
  );

describe("GuideJourney", () => {
  it("composes a ready controller snapshot into the shared Guide presentation", async () => {
    const controller = createGuideController({
      now: () => 900_000,
      source: { load: vi.fn().mockResolvedValue(sourceGuide) },
    });
    await controller.refresh("cartoons");

    const output = markup(controller);

    expect(output).toContain("Cartoon Classics");
    expect(output).toContain("The Borrowed Teapot");
    expect(output).toContain('accessibilityLabel="Programme guide"');
  });

  it("counts the person's filters over the served guide, and keeps an empty one off", async () => {
    const controller = createGuideController({
      now: () => 900_000,
      source: { load: vi.fn().mockResolvedValue(sourceGuide) },
    });
    await controller.refresh("cartoons");
    const render = (myChannels?: { favouriteIds: string[]; recentIds: string[] }) =>
      renderToStaticMarkup(
        <LoomarrProvider>
          <GuideJourney controller={controller} myChannels={myChannels} onTune={vi.fn()} />
        </LoomarrProvider>,
      );

    // A starred channel the guide no longer serves isn't counted.
    const output = render({ favouriteIds: ["cartoons", "retired"], recentIds: [] });
    expect(output).toContain(">All · 1<");
    expect(output).toContain(">★ Favorites · 1<");
    expect(output).toContain(">Recent · 0<");
    expect(output).toContain('aria-label="Recent, 0 channels"');
    expect(output.match(/aria-disabled="true"/g)).toHaveLength(1);

    // Before the lists arrive, both personal filters are off and uncounted.
    const unknown = render();
    expect(unknown).toContain(">★ Favorites<");
    expect(unknown.match(/aria-disabled="true"/g)).toHaveLength(2);
  });

  it("lets a platform adapter bound the rendered Channel rows", async () => {
    const controller = createGuideController({
      now: () => 900_000,
      source: { load: vi.fn().mockResolvedValue(sourceGuide) },
    });
    await controller.refresh("cartoons");

    const output = renderToStaticMarkup(
      <LoomarrProvider>
        <GuideJourney
          channelWindow={() => ({ end: 0, positionLabel: "1 of 1", start: 0 })}
          controller={controller}
          onTune={vi.fn()}
        />
      </LoomarrProvider>,
    );

    expect(output).not.toContain("Cartoon Classics, Kettle Kids");
    expect(output).toContain("1 channels · 1 of 1");
  });

  it("maps loading, empty, and error controller states without platform input mechanics", async () => {
    const loading = createGuideController({ source: { load: vi.fn() } });
    expect(markup(loading)).toContain("Loading channels");

    const empty = createGuideController({
      source: { load: vi.fn().mockResolvedValue({ ...sourceGuide, channels: [] }) },
    });
    await empty.refresh();
    expect(markup(empty)).toContain("No channels on air");

    const failed = createGuideController({
      source: { load: vi.fn().mockRejectedValue(new Error("offline")) },
    });
    await failed.refresh();
    expect(markup(failed)).toContain("Guide unavailable");
    expect(markup(failed)).toContain("Try again");
  });

  it("says so when a refresh fails over a shown Guide, and drops it once a refresh works", async () => {
    const load = vi
      .fn()
      .mockResolvedValueOnce(sourceGuide)
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValue(sourceGuide);
    const controller = createGuideController({ now: () => 900_000, source: { load } });
    await controller.refresh("cartoons");
    expect(markup(controller)).not.toContain("Couldn't refresh the Guide");

    // Mounting refreshes the Guide; that read fails, so the grid stays but is no longer current.
    const container = doc.createElement("div");
    doc.body.append(container);
    const root = createRoot(container);
    await act(async () => {
      root.render(
        <LoomarrProvider>
          <GuideJourney controller={controller} onTune={vi.fn()} preferredChannelId="cartoons" />
        </LoomarrProvider>,
      );
    });

    const notice = doc.body.querySelector('[role="status"]');
    expect(notice?.textContent).toContain("Couldn't refresh the Guide");
    expect(notice?.textContent).toContain("Showing what Loomarr had a moment ago.");
    expect(doc.body.textContent).toContain("Cartoon Classics");

    const before = load.mock.calls.length;
    const retry = Array.from(doc.body.querySelectorAll('[role="button"]')).find(
      (node) => node.textContent?.trim() === "Try again",
    );
    await act(async () => {
      retry?.click();
    });

    expect(load.mock.calls.length).toBe(before + 1);
    expect(doc.body.textContent).not.toContain("Couldn't refresh the Guide");
    expect(controller.getSnapshot().error).toBeUndefined();
  });
});
