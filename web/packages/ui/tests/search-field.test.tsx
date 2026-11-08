// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act, type ReactNode, useState } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it } from "vitest";

import { SearchField } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// This package has no DOM lib (it targets React Native too), so the slice of the jsdom document
// the test touches is typed here.
type Input = { getAttribute(name: string): string | null; value: string };
type Doc = {
  body: {
    append(node: unknown): void;
    querySelector(s: string): Input | null;
    querySelectorAll(s: string): ArrayLike<unknown>;
    replaceChildren(): void;
    textContent: string | null;
  };
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

let latest = "";
const Controlled = () => {
  const [query, setQuery] = useState("Mo");
  latest = query;
  return <SearchField label="Search people" onChangeText={setQuery} placeholder="Name" value={query} />;
};

describe("SearchField", () => {
  it("is one input named by its visible label, with a decorative magnifier", async () => {
    await mount(<Controlled />);

    const input = doc.body.querySelector("input");
    expect(input?.getAttribute("aria-label")).toBe("Search people");
    expect(input?.value).toBe("Mo");
    expect(latest).toBe("Mo");
    expect(doc.body.textContent).toContain("Search people");
    // The glyph is the only image, and it is hidden from assistive tech.
    expect(doc.body.querySelectorAll('[role="img"]').length).toBe(0);
  });
});
