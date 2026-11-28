// @vitest-environment jsdom

import { act, cleanup, render } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { LoomarrProvider } from "../index";
import { ListGroup, ListRow } from "../rows";

const doc = document;

const mount = async (node: ReactNode) => {
  await act(async () => {
    render(<LoomarrProvider theme="dark">{node}</LoomarrProvider>);
  });
};
afterEach(cleanup);

describe("ListRow", () => {
  it("is announced as a list of rows", async () => {
    await mount(
      <ListGroup aria-label="On the way">
        <ListRow sub="For the late-night channel" title="A sci-fi anthology" tone="progress" />
        <ListRow title="A horror anthology" tone="progress" />
      </ListGroup>,
    );

    const list = doc.body.querySelector('[role="list"]');
    expect(list?.getAttribute("aria-label")).toBe("On the way");
    expect(list?.querySelectorAll('[role="listitem"]').length).toBe(2);
  });

  it("names its progress bar and shows the eta beside it", async () => {
    await mount(
      <ListGroup>
        <ListRow
          progress={{ eta: "about 40 min", label: "Downloading", value: 22 }}
          title="A sci-fi anthology"
          tone="progress"
        />
      </ListGroup>,
    );

    const bar = doc.body.querySelector('[role="progressbar"]');
    expect(bar?.getAttribute("aria-label")).toBe("Downloading");
    expect(bar?.getAttribute("aria-valuenow")).toBe("22");
    expect(doc.body.textContent).toContain("about 40 min");
  });

  // axe `nested-interactive` failed 45 nodes in the mock (#1659): cards and rows that were buttons
  // holding buttons. The row is never interactive, so its action is the only control in it.
  it("keeps its action as the row's only control", async () => {
    await mount(
      <ListGroup>
        <ListRow
          action={<button type="button">Edit and retry</button>}
          sub="Couldn't be built: only 2 titles found"
          title="A game-show request"
          tone="error"
        />
      </ListGroup>,
    );

    const row = doc.body.querySelector('[role="listitem"]');
    expect(row?.querySelectorAll("button").length).toBe(1);
    expect(row?.closest("button")).toBeNull();
  });

  it("hides the dot unless it adds something the title does not say", async () => {
    await mount(
      <ListGroup>
        <ListRow title="A mystery channel" tone="attention" />
        <ListRow title="A space channel" tone="attention" toneLabel="Waiting for an admin" />
      </ListGroup>,
    );

    const dots = doc.body.querySelectorAll('[role="img"]');
    expect(dots.length).toBe(1);
    expect(dots[0]?.getAttribute("aria-label")).toBe("Waiting for an admin");
  });
});
