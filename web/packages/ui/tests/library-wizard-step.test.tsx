// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

import { type LibraryFolder, LibraryWizardStep, type LibraryWizardStepProps } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type El = { click(): void; textContent: string | null };
type Host = Parameters<typeof createRoot>[0] & {
  querySelectorAll(s: string): ArrayLike<El>;
  remove(): void;
  textContent: string | null;
};
const doc = (
  globalThis as unknown as {
    document: { body: { append(n: unknown): void }; createElement(t: string): Host };
  }
).document;

const TV: LibraryFolder = {
  path: "/media/tv",
  inUse: false,
  readable: true,
  checkedAt: "2026-11-02T11:48:00Z",
  addedByHand: false,
  foundIn: [{ authority: "sonarr", path: "/tv" }],
};

const render = (overrides: Partial<LibraryWizardStepProps>) => {
  const props: LibraryWizardStepProps = {
    folders: [TV],
    pathMap: [
      { authority: "emby", authorityPath: "/data/tv", localPath: "/media/tv", byHand: false },
      { authority: "sonarr", authorityPath: "/tv", localPath: "/media/tv", byHand: false },
    ],
    scan: { enabled: true, stage: "idle", headersDone: 0, headersTotal: 0, paused: false },
    checking: false,
    onCheckAgain: vi.fn(),
    onUseFolder: vi.fn(),
    onAddFolder: vi.fn(async () => undefined),
    now: Date.parse("2026-11-02T12:00:00Z"),
    ...overrides,
  };
  const host = doc.createElement("div");
  doc.body.append(host);
  const root = createRoot(host);
  act(() =>
    root.render(
      <LoomarrProvider theme="dark">
        <LibraryWizardStep {...props} />
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

describe("LibraryWizardStep", () => {
  it("says where the folders were found, and uses one", () => {
    const { host, props, button, cleanup } = render({});
    expect(host.textContent).toContain(
      "We found these through Emby and Sonarr — pick the ones that hold your library.",
    );
    expect(host.textContent).toContain("Make them available to Loomarr");
    act(() => button("Use this folder").click());
    expect(props.onUseFolder).toHaveBeenCalledWith("/media/tv", true);
    cleanup();
  });

  it("shows the import going once a folder is in use, and doesn't hold the step for it", () => {
    const { host, cleanup } = render({
      folders: [{ ...TV, inUse: true }],
      scan: { enabled: true, stage: "reading", headersDone: 25, headersTotal: 100, paused: false },
    });
    expect(host.textContent).toContain("Adding your media");
    expect(host.textContent).toContain("You can carry on — titles appear as they're ready.");
    expect(host.textContent).not.toContain("Make them available to Loomarr");
    cleanup();
  });

  it("checks the folders again, and says when it just did", () => {
    const { host, props, button, cleanup } = render({ checkedJustNow: true });
    act(() => button("Check again").click());
    expect(props.onCheckAgain).toHaveBeenCalled();
    expect(host.textContent).toContain("Checked just now");
    cleanup();
  });

  it("asks for a folder when none was found", () => {
    const { host, button, cleanup } = render({ folders: [], pathMap: [] });
    expect(host.textContent).toContain("Loomarr didn't find any folders yet.");
    expect(button("Add a folder")).toBeTruthy();
    cleanup();
  });
});
