// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ChannelGrowthSection, type ChannelGrowthSectionProps } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type Node = { click(): void; getAttribute(n: string): string | null; textContent: string | null };
type Host = Parameters<typeof createRoot>[0] & {
  querySelector(s: string): Node | null;
  querySelectorAll(s: string): ArrayLike<Node>;
  textContent: string | null;
};
type Doc = {
  body: { append(n: unknown): void; replaceChildren(): void };
  createElement(t: string): Host;
};
const doc = (globalThis as unknown as { document: Doc }).document;

const NONE = { episodes: 0, films: 0, shows: 0 };

const props = (over: Partial<ChannelGrowthSectionProps> = {}): ChannelGrowthSectionProps => ({
  joined: [],
  keep: false,
  lineWords: "series from 1986 to 1995, rated up to TV-PG, comedy and family",
  off: [],
  onAddBack: vi.fn(),
  onKeepChange: vi.fn(),
  onTakeOff: vi.fn(),
  short: NONE,
  still: "",
  target: { episodes: 400, films: 0, shows: 8 },
  waiting: 0,
  ...over,
});

const mount = async (p: ChannelGrowthSectionProps) => {
  const host = doc.createElement("div");
  doc.body.append(host);
  const root = createRoot(host);
  await act(async () => {
    root.render(
      <LoomarrProvider theme="dark">
        <ChannelGrowthSection {...p} />
      </LoomarrProvider>,
    );
  });
  const button = (name: string) => {
    const found = Array.from(host.querySelectorAll("button, [role='button']")).find(
      (n) => n.getAttribute("aria-label") === name,
    );
    if (!found) throw new Error(`no button named ${name}`);
    return found;
  };
  return { button, host };
};
afterEach(() => doc.body.replaceChildren());

describe("ChannelGrowthSection", () => {
  it("says what the channel adds, in the line's own words", async () => {
    const { host } = await mount(props());
    expect(host.querySelector('[role="region"]')?.getAttribute("aria-label")).toBe("Grows by itself");
    expect(host.textContent).toContain("series from 1986 to 1995, rated up to TV-PG, comedy and family");
  });

  it.each([
    ["kept", "Kept as it is, so nothing joins by itself."],
    ["by_hand", "Made by hand, so it doesn't add shows by itself."],
    ["fixed", "Nothing in your library fits its picks yet."],
    ["not_active", "Not on air, so it isn't adding anything."],
  ] as const)("says why a %s channel doesn't grow", async (still, sentence) => {
    const { host } = await mount(props({ still }));
    expect(host.textContent).toContain(sentence);
  });

  it("gives no reason when the channel grows", async () => {
    const { host } = await mount(props({ still: "" }));
    expect(host.textContent).not.toContain("doesn't add");
    expect(host.textContent).not.toContain("isn't adding");
  });

  it("says what it still needs for a week without repeats", async () => {
    const { host } = await mount(props({ short: { episodes: 120, films: 0, shows: 2 } }));
    expect(host.textContent).toContain(
      "Needs about 120 more episodes and 2 more shows for a week without repeats.",
    );
  });

  it("lists every kind that is short, with singulars", async () => {
    const { host } = await mount(props({ short: { episodes: 1, films: 3, shows: 1 } }));
    expect(host.textContent).toContain(
      "Needs about 1 more episode, 1 more show and 3 more films for a week without repeats.",
    );
  });

  it("says it has enough when nothing is short", async () => {
    const { host } = await mount(props());
    expect(host.textContent).toContain("It has enough for a week without repeats.");
  });

  it("says nothing about a shortfall when there is no target", async () => {
    const { host } = await mount(props({ target: NONE }));
    expect(host.textContent).not.toContain("week without repeats");
  });

  it("says when titles are waiting for the AI check", async () => {
    const one = await mount(props({ waiting: 1 }));
    expect(one.host.textContent).toContain("1 title is waiting for the AI check before it can join.");
    const many = await mount(props({ waiting: 4 }));
    expect(many.host.textContent).toContain("4 titles are waiting for the AI check before they can join.");
  });

  it("groups what joined by night, newest first, and hands a take-off back", async () => {
    const onTakeOff = vi.fn();
    const { button, host } = await mount(
      props({
        joined: [
          { batch: "growth-2026-11-16", key: "series:tvdb:90000001", name: "Harbor Lights" },
          { batch: "growth-2026-11-18", key: "series:tvdb:90000002", name: "Night Desk" },
          { batch: "growth-2026-11-18", key: "movie:tmdb:90000003", name: "The Long Weekend" },
        ],
        onTakeOff,
      }),
    );
    const labels = Array.from(host.querySelectorAll('[role="list"]')).map((n) =>
      n.getAttribute("aria-label"),
    );
    expect(labels).toEqual(["Added on November 18, 2026", "Added on November 16, 2026"]);

    await act(async () => button("Take off Night Desk").click());
    expect(onTakeOff).toHaveBeenCalledWith("series:tvdb:90000002");
  });

  it("shows the model's reason for a title it took off, and says so when a person did", async () => {
    const onAddBack = vi.fn();
    const { button, host } = await mount(
      props({
        off: [
          { key: "series:tvdb:90000004", name: "Quiet Harbor", reason: "Too grown-up for this channel" },
          { key: "series:tvdb:90000005", name: "Paper Moon Diner", reason: "" },
        ],
        onAddBack,
      }),
    );
    expect(host.textContent).toContain("Too grown-up for this channel");
    expect(host.textContent).toContain("Taken off by hand");

    await act(async () => button("Add back Quiet Harbor").click());
    expect(onAddBack).toHaveBeenCalledWith("series:tvdb:90000004");
  });

  it("flips the Keep as it is switch", async () => {
    const onKeepChange = vi.fn();
    const { host } = await mount(props({ onKeepChange }));
    const toggle = host.querySelector('[role="switch"]');
    expect(toggle?.getAttribute("aria-checked")).toBe("false");
    await act(async () => toggle?.click());
    expect(onKeepChange).toHaveBeenCalledWith(true);
  });
});
