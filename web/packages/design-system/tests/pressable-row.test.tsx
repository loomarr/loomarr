// @vitest-environment jsdom
import { act } from "react";
import { createRoot } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { LoomarrProvider, PressableRow, Text } from "../index";

const markupOf = (node: React.ReactNode) =>
  renderToStaticMarkup(<LoomarrProvider theme="dark">{node}</LoomarrProvider>);

describe("PressableRow", () => {
  it("is one button named by its content", () => {
    const markup = markupOf(
      <PressableRow>
        <Text textRole="body">Morning Cartoons</Text>
      </PressableRow>,
    );
    expect(markup).toContain('role="button"');
    expect(markup).toContain("Morning Cartoons");
  });

  it("takes the role the caller names", () => {
    expect(markupOf(<PressableRow accessibilityRole="option">Austin</PressableRow>)).toContain(
      'role="option"',
    );
  });

  it("draws the current row differently from the rest", () => {
    expect(markupOf(<PressableRow selected>A</PressableRow>)).not.toEqual(
      markupOf(<PressableRow>A</PressableRow>),
    );
  });

  it("announces selected for an option but not for a plain button", () => {
    expect(
      markupOf(
        <PressableRow accessibilityRole="option" selected>
          A
        </PressableRow>,
      ),
    ).toContain('aria-selected="true"');
    expect(markupOf(<PressableRow selected>A</PressableRow>)).not.toContain("aria-selected");
  });

  it("draws a hairline only when divided", () => {
    expect(markupOf(<PressableRow divided>A</PressableRow>)).not.toEqual(
      markupOf(<PressableRow>A</PressableRow>),
    );
  });

  it("forwards mousedown so a combobox option can keep the input focused", () => {
    const container = document.createElement("div");
    document.body.append(container);
    const root = createRoot(container);
    const input = document.createElement("input");
    document.body.append(input);
    act(() => {
      root.render(
        <LoomarrProvider theme="dark">
          <PressableRow accessibilityRole="option" onMouseDown={(event) => event.preventDefault()}>
            Austin
          </PressableRow>
        </LoomarrProvider>,
      );
    });
    input.focus();
    const option = container.querySelector<HTMLElement>('[role="option"]');
    const mousedown = new MouseEvent("mousedown", { bubbles: true, cancelable: true });
    act(() => {
      option?.dispatchEvent(mousedown);
    });
    expect(mousedown.defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(input);
    act(() => root.unmount());
    container.remove();
    input.remove();
  });

  describe("rendered as a link", () => {
    const mount = (ui: React.ReactNode) => {
      const container = document.createElement("div");
      document.body.append(container);
      const root = createRoot(container);
      act(() => {
        root.render(<LoomarrProvider theme="dark">{ui}</LoomarrProvider>);
      });
      return {
        container,
        unmount: () => {
          act(() => root.unmount());
          container.remove();
        },
      };
    };

    it("is the anchor itself, with its address and no button role", () => {
      const { container, unmount } = mount(
        <PressableRow render={<a href="/guide" />}>
          <Text textRole="body">Guide</Text>
        </PressableRow>,
      );
      const link = container.querySelector<HTMLAnchorElement>("a");
      expect(link?.getAttribute("href")).toBe("/guide");
      expect(link?.getAttribute("role")).toBeNull();
      expect(link?.textContent).toBe("Guide");
      expect(container.querySelector('[role="button"]')).toBeNull();
      unmount();
    });

    it("draws the hover fill and the focus border as a button row does", () => {
      const { container, unmount } = mount(<PressableRow render={<a href="/guide" />}>Guide</PressableRow>);
      const link = container.querySelector<HTMLAnchorElement>("a") as HTMLAnchorElement;
      const markup = () => link.innerHTML;
      const resting = markup();

      act(() => {
        link.dispatchEvent(new MouseEvent("mouseover", { bubbles: true }));
        link.dispatchEvent(new MouseEvent("mouseout", { bubbles: true }));
      });
      expect(markup()).toBe(resting);

      act(() => {
        link.focus();
      });
      expect(markup()).not.toBe(resting);
      act(() => {
        link.blur();
      });
      expect(markup()).toBe(resting);
      unmount();
    });

    it("keeps the element's own click, and chains onPress after it", () => {
      const order: string[] = [];
      const { container, unmount } = mount(
        <PressableRow
          onPress={() => order.push("press")}
          render={
            <a
              href="/guide"
              onClick={(event) => {
                event.preventDefault();
                order.push("link");
              }}
            />
          }
        >
          Guide
        </PressableRow>,
      );
      act(() => {
        container.querySelector<HTMLAnchorElement>("a")?.click();
      });
      expect(order).toEqual(["link", "press"]);
      unmount();
    });

    it("marks the current row as the current page", () => {
      const { container, unmount } = mount(
        <PressableRow accessibilityRole="link" render={<a href="/guide" />} selected>
          Guide
        </PressableRow>,
      );
      expect(container.querySelector("a")?.getAttribute("aria-current")).toBe("page");
      unmount();
    });

    it("hands a function child and a function selected the link's state, as a router Link gives it", () => {
      // A stand-in for a router Link: it calls a function child with its own `isActive`.
      const RouterLink = ({
        children,
        isActive,
        ...props
      }: {
        children?: (state: { isActive: boolean }) => React.ReactNode;
        isActive: boolean;
      } & React.AnchorHTMLAttributes<HTMLAnchorElement>) => <a {...props}>{children?.({ isActive })}</a>;
      const row = (isActive: boolean) => (
        <PressableRow
          render={<RouterLink href="/guide" isActive={isActive} />}
          selected={({ isActive: active }) => active}
        >
          {({ isActive: active }) => (active ? "Guide, here" : "Guide")}
        </PressableRow>
      );

      const here = mount(row(true));
      expect(here.container.querySelector("a")?.textContent).toBe("Guide, here");
      here.unmount();

      const away = mount(row(false));
      expect(away.container.querySelector("a")?.textContent).toBe("Guide");
      away.unmount();

      // The fill follows `selected`: the active row draws differently from the idle one.
      const filled = mount(row(true));
      const idle = mount(row(false));
      expect(filled.container.querySelector("a")?.innerHTML).not.toBe(
        idle.container.querySelector("a")?.innerHTML.replace("Guide", "Guide, here"),
      );
      filled.unmount();
      idle.unmount();
    });

    it("leaves a disabled link out of the tab order and ignores its press", () => {
      const onPress = vi.fn();
      const { container, unmount } = mount(
        <PressableRow disabled onPress={onPress} render={<a href="/guide" />}>
          Guide
        </PressableRow>,
      );
      const link = container.querySelector<HTMLAnchorElement>("a");
      expect(link?.getAttribute("aria-disabled")).toBe("true");
      expect(link?.tabIndex).toBe(-1);
      act(() => link?.click());
      expect(onPress).not.toHaveBeenCalled();
      unmount();
    });
  });

  it("runs onPress, and not when disabled", () => {
    const onPress = vi.fn();
    const container = document.createElement("div");
    document.body.append(container);
    const root = createRoot(container);
    act(() => {
      root.render(
        <LoomarrProvider theme="dark">
          <PressableRow onPress={onPress}>Open</PressableRow>
          <PressableRow disabled onPress={onPress}>
            Shut
          </PressableRow>
        </LoomarrProvider>,
      );
    });
    const [first, second] = [...container.querySelectorAll<HTMLElement>('[role="button"]')];
    act(() => {
      first?.click();
      second?.click();
    });
    expect(onPress).toHaveBeenCalledTimes(1);
    act(() => root.unmount());
    container.remove();
  });
});
