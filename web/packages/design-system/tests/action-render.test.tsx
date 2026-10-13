// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Action, LoomarrProvider } from "../index";

describe("Action variants", () => {
  // react-native-web flattens the Pressable's own style object to inline rgba() (not hex, and
  // "transparent" becomes rgba(0,0,0,0.00)) — only the Tamagui-rendered Text child keeps literal
  // hex, since that goes through Tamagui's own atomic-CSS resolution instead.
  it("renders suggest/destructive with dark text on a solid accent", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Action variant="suggest">Suggest a lineup</Action>
      </LoomarrProvider>,
    );
    // brandChroma[4], the base suggest magenta — not accent.suggest's lighter -300 stop, which
    // is calibrated for TEXT on a neutral ground, not for a fill a dark label sits on top of.
    expect(markup).toContain("background-color:rgba(214,64,159,1.00)");
    expect(markup).toContain("color:#0B0C0E");
  });

  it("renders outline with a border and no fill", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Action variant="outline">Cancel</Action>
      </LoomarrProvider>,
    );
    expect(markup).toContain("background-color:rgba(0,0,0,0.00)");
    expect(markup).toContain("border-top-color:rgba(102,105,112,1.00)");
  });

  it("renders link as underlined info-coloured text with no box", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Action variant="link">Read the docs</Action>
      </LoomarrProvider>,
    );
    expect(markup).toContain("background-color:rgba(0,0,0,0.00)");
    expect(markup).toContain("text-decoration-line:underline");
  });
});

describe("Action render composition", () => {
  let host: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
    host = document.createElement("div");
    document.body.append(host);
    root = createRoot(host);
  });

  afterEach(() => {
    act(() => root.unmount());
    host.remove();
  });

  it("renders as the supplied element instead of a Pressable", () => {
    act(() => {
      root.render(
        <LoomarrProvider>
          <Action render={<a href="/channels" />} variant="outline">
            Go
          </Action>
        </LoomarrProvider>,
      );
    });

    const link = host.querySelector("a");
    expect(link).not.toBeNull();
    expect(link?.getAttribute("href")).toBe("/channels");
    expect(link?.textContent).toBe("Go");
  });

  // Mirrors the legacy Button's own `render` contract test: neither side's className is
  // dropped, and both click handlers fire.
  it("keeps both sides' className and chains both click handlers", () => {
    const ownClick = vi.fn();
    // `onPress` is Action's own (Pressable) handler name; the render path bridges it to the
    // cloned element's click so a caller does not have to know the host swapped.
    const actionPress = vi.fn();
    act(() => {
      root.render(
        <LoomarrProvider>
          <Action
            onPress={actionPress}
            render={<a className="custom" href="/channels" onClick={ownClick} />}
            variant="outline"
          >
            Go
          </Action>
        </LoomarrProvider>,
      );
    });

    const link = host.querySelector("a") as HTMLAnchorElement;
    expect(link.className).toContain("custom");

    act(() => {
      link.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
    });
    expect(ownClick).toHaveBeenCalledOnce();
    expect(actionPress).toHaveBeenCalledOnce();
  });

  it("is ignored on children that are not a valid element", () => {
    // Guards the isValidElement check: a Pressable still renders when `render` isn't usable.
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Action render={undefined} variant="secondary">
          Save
        </Action>
      </LoomarrProvider>,
    );
    expect(markup).toContain("Save");
  });
});
