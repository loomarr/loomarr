// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

import { type LibraryFolder, LibrarySettings, type LibrarySettingsProps } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type El = {
  click(): void;
  dispatchEvent(e: unknown): boolean;
  getAttribute(n: string): string | null;
  textContent: string | null;
  value: string;
};
type Host = Parameters<typeof createRoot>[0] & {
  querySelector(s: string): El | null;
  querySelectorAll(s: string): ArrayLike<El>;
  remove(): void;
  textContent: string | null;
};
const doc = (
  globalThis as unknown as {
    document: { body: { append(n: unknown): void }; createElement(t: string): Host };
  }
).document;

const NOW = Date.parse("2026-11-02T12:00:00Z");
const TV: LibraryFolder = {
  path: "/media/tv",
  inUse: false,
  readable: true,
  checkedAt: "2026-11-02T11:48:00Z",
  addedByHand: false,
  foundIn: [
    { authority: "emby", path: "/data/tv" },
    { authority: "sonarr", path: "/tv" },
  ],
};
const MOVIES: LibraryFolder = { ...TV, path: "/media/movies", inUse: true, readable: false, foundIn: [] };

const render = (overrides: Partial<LibrarySettingsProps>) => {
  const props: LibrarySettingsProps = {
    health: "needs_folders",
    folders: [TV],
    pathMap: [{ authority: "sonarr", authorityPath: "/tv", localPath: "/media/tv", byHand: false }],
    checking: false,
    onCheckAgain: vi.fn(),
    onUseFolder: vi.fn(),
    onAddFolder: vi.fn(async () => "Loomarr can't open that folder"),
    onMapFolder: vi.fn(),
    now: NOW,
    ...overrides,
  };
  const host = doc.createElement("div");
  doc.body.append(host);
  const root = createRoot(host);
  act(() =>
    root.render(
      <LoomarrProvider theme="dark">
        <LibrarySettings {...props} />
      </LoomarrProvider>,
    ),
  );
  const button = (name: string) => {
    const hit = Array.from(host.querySelectorAll('[role="button"], button')).find(
      (n) => n.textContent?.trim() === name,
    );
    if (!hit) throw new Error(`missing button: ${name}`);
    return hit;
  };
  const cleanup = () => {
    act(() => root.unmount());
    host.remove();
  };
  return { host, props, button, cleanup };
};

const type = (input: El, text: string) => {
  const setter = Object.getOwnPropertyDescriptor(
    (globalThis as unknown as { HTMLInputElement: { prototype: object } }).HTMLInputElement.prototype,
    "value",
  )?.set;
  act(() => {
    setter?.call(input, text);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
};

describe("LibrarySettings", () => {
  it("asks for folders in plain words and confirms one", () => {
    const { host, props, button, cleanup } = render({});
    expect(host.textContent).toContain("Tell Loomarr where your media is");
    expect(host.textContent).toContain("Found through Emby and Sonarr · Checked 12 minutes ago");
    expect(host.textContent).toContain("Loomarr can open this folder");
    act(() => button("Use this folder").click());
    expect(props.onUseFolder).toHaveBeenCalledWith("/media/tv", true);
    cleanup();
  });

  it("names an offline folder with one way out", () => {
    const { host, props, button, cleanup } = render({ health: "folder_offline", folders: [MOVIES] });
    expect(host.textContent).toContain("Can't reach your Movies folder");
    expect(host.textContent).toContain("Loomarr can't open this folder right now");
    expect(host.textContent).toContain("No app names this folder any more");
    act(() => button("Stop using").click());
    expect(props.onUseFolder).toHaveBeenCalledWith("/media/movies", false);
    act(() =>
      Array.from(host.querySelectorAll("button, [role=button]"))
        .find((n) => n.textContent === "Check again")
        ?.click(),
    );
    expect(props.onCheckAgain).toHaveBeenCalled();
    cleanup();
  });

  it("keeps a refused folder in the field with the reason", async () => {
    const { host, props, button, cleanup } = render({});
    act(() => button("Add a folder").click());
    const input = host.querySelector('input[aria-label="Where is the folder?"], input') as El;
    type(input, "/mnt/missing");
    await act(async () => button("Add folder").click());
    expect(props.onAddFolder).toHaveBeenCalledWith("/mnt/missing");
    expect(host.textContent).toContain("Loomarr can't open that folder");
    expect(input.value).toBe("/mnt/missing");
    cleanup();
  });
});
