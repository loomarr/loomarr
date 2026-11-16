// @vitest-environment jsdom
import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Details, LoomarrProvider } from "../index";

afterEach(cleanup);

const inTheme = (ui: React.ReactNode) => <LoomarrProvider theme="dark">{ui}</LoomarrProvider>;

const detailsOf = (container: HTMLElement) => container.querySelector("details") as HTMLDetailsElement;

describe("Details", () => {
  it("is a native details with the summary first, closed unless told otherwise", () => {
    const { container } = render(inTheme(<Details summary="Technical details">The body</Details>));

    const details = detailsOf(container);
    expect(details.tagName).toBe("DETAILS");
    expect(details.open).toBe(false);
    expect(details.firstElementChild?.tagName).toBe("SUMMARY");
    expect(details.firstElementChild?.textContent).toBe("Technical details");
    expect(screen.getByText("The body").closest("details")).toBe(details);
  });

  it("draws the bare look: a native marker, a pointer, and the text role's own type", () => {
    const { container } = render(
      inTheme(
        <Details summary="More filters" textRole="compact" tone="secondary">
          Body
        </Details>,
      ),
    );

    const summary = container.querySelector("summary") as HTMLElement;
    expect(summary.style.cursor).toBe("pointer");
    // No list-style override, so the browser's own marker shows.
    expect(summary.style.listStyle).toBe("");
    expect(summary.style.width).toBe("");
    expect(summary.style.fontSize).toBe("13px");
    expect(summary.style.color).not.toBe("");
  });

  it("is reachable by Tab, and reports each open and close", async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();
    const { container } = render(
      inTheme(
        <Details onToggle={onToggle} summary="Raw search data">
          Body
        </Details>,
      ),
    );
    const details = detailsOf(container);
    const summary = container.querySelector("summary") as HTMLElement;

    // The summary is the browser's own focusable control, so Enter and Space open it with no
    // handler of ours (jsdom does not activate a summary from a key, so the press is a click).
    await user.tab();
    expect(summary).toBe(document.activeElement);
    await user.click(summary);
    expect(details.open).toBe(true);
    expect(onToggle).toHaveBeenLastCalledWith(true);

    await user.click(summary);
    expect(details.open).toBe(false);
    expect(onToggle).toHaveBeenLastCalledWith(false);
    expect(onToggle).toHaveBeenCalledTimes(2);
  });

  it("starts open with defaultOpen and lets the reader close it", async () => {
    const user = userEvent.setup();
    const { container } = render(
      inTheme(
        <Details defaultOpen summary="Details">
          Body
        </Details>,
      ),
    );
    const details = detailsOf(container);
    expect(details.open).toBe(true);

    await user.click(container.querySelector("summary") as HTMLElement);
    expect(details.open).toBe(false);
  });

  it("follows `open` when the page changes it, and keeps the reader's toggle until then", async () => {
    const user = userEvent.setup();
    const view = render(
      inTheme(
        <Details open={false} summary="Rename or adjust timing">
          Body
        </Details>,
      ),
    );
    const details = detailsOf(view.container);
    expect(details.open).toBe(false);

    await user.click(view.container.querySelector("summary") as HTMLElement);
    expect(details.open).toBe(true);

    // The same prop again is not a change: the reader's open stands.
    view.rerender(
      inTheme(
        <Details open={false} summary="Rename or adjust timing">
          Body
        </Details>,
      ),
    );
    expect(detailsOf(view.container).open).toBe(true);

    view.rerender(
      inTheme(
        <Details open summary="Rename or adjust timing">
          Body
        </Details>,
      ),
    );
    expect(detailsOf(view.container).open).toBe(true);

    view.rerender(
      inTheme(
        <Details open={false} summary="Rename or adjust timing">
          Body
        </Details>,
      ),
    );
    expect(detailsOf(view.container).open).toBe(false);
  });

  it("hands a function summary the open state, so the label swaps with it", async () => {
    const user = userEvent.setup();
    const { container } = render(
      inTheme(
        <Details summary={(open) => (open ? "Hide error" : "Show error")} tone="danger">
          Boom
        </Details>,
      ),
    );
    const summary = container.querySelector("summary") as HTMLElement;
    expect(summary.textContent).toBe("Show error");

    await user.click(summary);
    expect(summary.textContent).toBe("Hide error");

    await user.click(summary);
    expect(summary.textContent).toBe("Show error");
  });

  it("takes the marker away and hugs the words when asked", () => {
    const { container } = render(
      inTheme(
        <Details fitSummary marker={false} summary="Customize report">
          Body
        </Details>,
      ),
    );

    const summary = container.querySelector("summary") as HTMLElement;
    expect(summary.style.listStyle).toBe("none");
    expect(summary.style.width).toBe("fit-content");
  });

  it("draws the amber focus outline for a keyboard reader and not otherwise", async () => {
    const user = userEvent.setup();
    const { container } = render(inTheme(<Details summary="Details">Body</Details>));
    const summary = container.querySelector("summary") as HTMLElement;
    expect(summary.style.outline).toBe("none");

    // jsdom has no :focus-visible, so stand in for the browser's answer.
    const matches = vi
      .spyOn(summary, "matches")
      .mockImplementation((selector) => selector === ":focus-visible");
    await act(async () => summary.focus());
    expect(summary.style.outline).toContain("2px solid");

    await act(async () => summary.blur());
    expect(summary.style.outline).toBe("none");

    matches.mockImplementation(() => false);
    await user.click(summary);
    expect(summary.style.outline).toBe("none");
  });
});
