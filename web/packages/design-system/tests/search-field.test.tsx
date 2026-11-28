// @vitest-environment jsdom

import { act, cleanup, render } from "@testing-library/react";
import { type ReactNode, useState } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { SearchField } from "../fields";
import { LoomarrProvider } from "../index";

const mount = async (node: ReactNode) => {
  await act(async () => {
    render(<LoomarrProvider theme="dark">{node}</LoomarrProvider>);
  });
};
afterEach(cleanup);

let latest = "";
const Controlled = ({ clearLabel }: { clearLabel?: string }) => {
  const [query, setQuery] = useState("Mo");
  latest = query;
  return (
    <SearchField
      clearLabel={clearLabel}
      label="Search people"
      onChangeText={setQuery}
      placeholder="Name"
      value={query}
    />
  );
};

const input = () => document.querySelector<HTMLInputElement>("input");
const clearButton = () => document.querySelector<HTMLElement>('[aria-label="Clear search"]');

describe("SearchField", () => {
  it("is one input named by its visible label, with a decorative magnifier", async () => {
    await mount(<Controlled />);

    expect(input()?.getAttribute("aria-label")).toBe("Search people");
    expect(input()?.value).toBe("Mo");
    expect(latest).toBe("Mo");
    expect(document.body.textContent).toContain("Search people");
    // The glyph is the only image, and it is hidden from assistive tech.
    expect(document.querySelectorAll('[role="img"]').length).toBe(0);
    // No clear button unless one is named.
    expect(clearButton()).toBeNull();
  });

  it("empties the query from its named clear button, which then goes away", async () => {
    await mount(<Controlled clearLabel="Clear search" />);

    expect(clearButton()).not.toBeNull();
    await act(async () => clearButton()?.click());

    expect(latest).toBe("");
    expect(input()?.value).toBe("");
    expect(clearButton()).toBeNull();
  });
});
