// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { LoomarrProvider, Reveal, Text, useRevealOnHover } from "../index";

afterEach(cleanup);

const Row = () => {
  const { rowProps, shown } = useRevealOnHover();
  return (
    <div data-testid="row" {...rowProps}>
      <button type="button">First</button>
      <button type="button">Second</button>
      <Reveal data-testid="audit" shown={shown}>
        <Text textRole="hint">Changed by Maya, 2 days ago</Text>
      </Reveal>
    </div>
  );
};

const renderRow = () =>
  render(
    <LoomarrProvider>
      <Row />
    </LoomarrProvider>,
  );

const audit = () => screen.getByTestId("audit");
// Tamagui draws opacity as an atomic class in an injected sheet; read what the cascade resolves to.
const opacityOf = (element: HTMLElement) => getComputedStyle(element).opacity;

describe("Reveal and useRevealOnHover", () => {
  it("keeps the child in the tree but not drawn or pointable at rest", () => {
    renderRow();

    expect(audit().textContent).toBe("Changed by Maya, 2 days ago");
    expect(opacityOf(audit())).toBe("0");
    expect(getComputedStyle(audit()).pointerEvents).toBe("none");

    fireEvent.mouseEnter(screen.getByTestId("row"));
    expect(getComputedStyle(audit()).pointerEvents).toBe("auto");
  });

  it("shows on hover and hides on leave", () => {
    renderRow();

    fireEvent.mouseEnter(screen.getByTestId("row"));
    expect(opacityOf(audit())).toBe("1");

    fireEvent.mouseLeave(screen.getByTestId("row"));
    expect(opacityOf(audit())).toBe("0");
  });

  it("shows while focus is inside the row, and stays through a move between its controls", () => {
    renderRow();
    const first = screen.getByRole("button", { name: "First" });
    const second = screen.getByRole("button", { name: "Second" });

    fireEvent.focus(first);
    expect(opacityOf(audit())).toBe("1");

    // Focus moving to the neighbour is still the row holding focus.
    fireEvent.blur(first, { relatedTarget: second });
    expect(opacityOf(audit())).toBe("1");

    fireEvent.blur(second, { relatedTarget: null });
    expect(opacityOf(audit())).toBe("0");
  });

  it("fades with a transition unless the reader asks for less motion", () => {
    renderRow();

    expect(audit().style.transition).toContain("opacity");
  });
});
