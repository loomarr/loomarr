// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it } from "vitest";

import { ChannelUnairable } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// This package has no DOM lib (it targets React Native too), so the slice of the jsdom document
// the test touches is typed here.
type Node = { textContent: string | null };
type Doc = {
  body: Node & { append(node: unknown): void; replaceChildren(): void };
  createElement(tag: string): Parameters<typeof createRoot>[0];
};
const doc = (globalThis as unknown as { document: Doc }).document;

const mount = async (node: ReactNode) => {
  const container = doc.createElement("div");
  doc.body.append(container);
  const root = createRoot(container);
  await act(async () => {
    root.render(<LoomarrProvider theme="dark">{node}</LoomarrProvider>);
  });
};
afterEach(() => {
  doc.body.replaceChildren();
});

describe("ChannelUnairable", () => {
  it("names each title that can't air and why", async () => {
    await mount(
      <ChannelUnairable
        titles={[
          { key: "series:tvdb:90000012", reason: "no_episodes_in_seasons", title: "Early Seasons Only" },
          { key: "movie:tmdb:90000002", reason: "not_in_library", title: "Missing Film" },
        ]}
      />,
    );
    const text = doc.body.textContent ?? "";
    expect(text).toContain("2 titles can't air");
    expect(text).toContain("Early Seasons Only has no episodes in the seasons this channel asks for.");
    expect(text).toContain("Missing Film isn't in your library.");
  });

  it("shows nothing when every title airs", async () => {
    await mount(<ChannelUnairable titles={[]} />);
    expect(doc.body.textContent).toBe("");
  });
});
