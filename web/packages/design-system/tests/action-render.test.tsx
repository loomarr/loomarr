// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  Action,
  brandChroma,
  ChoiceGroup,
  LoomarrProvider,
  ModalLayer,
  semanticThemes,
  Toggle,
} from "../index";

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

  it("renders the danger tone as a neutral tile with danger text, never the on-air red fill", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Action tone="danger">Disconnect</Action>
      </LoomarrProvider>,
    );
    // surface.elevated #1B1E24 under state.danger #E85A5F text, as an iOS destructive alert button.
    expect(markup).toContain("background-color:rgba(27,30,36,1.00)");
    expect(markup).not.toContain("background-color:rgba(232,90,95,1.00)");
    expect(markup).toContain("color:#E85A5F");
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

describe("Action selected vs focused (#1887)", () => {
  let host: HTMLDivElement;
  let root: Root;
  const amber = "rgb(255, 176, 32)";
  const focusFill = "rgb(40, 44, 52)";
  const foreground = "rgb(231, 234, 240)";
  const button = () => host.querySelector<HTMLElement>('[role="button"]') as HTMLElement;

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

  // Neutral-pressed (#1909 Q1/Q2): the focus fill plus a 2px foreground border, never thinner
  // than an unselected neighbour's, so the current item reads from across a room.
  it.each(["pointer", "touch", "tv"] as const)(
    "marks a selected %s action neutral-pressed, and amber only once it has focus",
    (density) => {
      act(() => {
        root.render(
          <LoomarrProvider>
            <Action density={density} selected tone="secondary">
              1.2 s
            </Action>
          </LoomarrProvider>,
        );
      });

      expect(button().getAttribute("aria-pressed")).toBe("true");
      expect(button().style.backgroundColor).toBe(focusFill);
      expect(button().style.borderTopColor).toBe(foreground);
      expect(button().style.borderTopWidth).toBe("2px");

      act(() => button().focus());
      expect(button().style.backgroundColor).toBe(focusFill);
      expect(button().style.borderTopColor).toBe(amber);
      expect(button().style.borderTopWidth).toBe(density === "tv" ? "3px" : "4px");

      act(() => button().blur());
      expect(button().style.borderTopColor).toBe(foreground);
    },
  );

  // The light focus fill was a pale amber cream, so selection read amber (#1953).
  it("marks a selected light action with a neutral grey fill, and amber only once it has focus", () => {
    act(() => {
      root.render(
        <LoomarrProvider theme="light">
          <Action selected tone="secondary">
            1.2 s
          </Action>
        </LoomarrProvider>,
      );
    });

    expect(button().style.backgroundColor).toBe("rgb(210, 214, 222)");
    expect(button().style.borderTopColor).toBe("rgb(23, 25, 29)");

    act(() => button().focus());
    expect(button().style.backgroundColor).toBe("rgb(210, 214, 222)");
    expect(button().style.borderTopColor).toBe("rgb(138, 87, 0)");
  });

  it("sets a selected tv label at 600 over its 400 neighbours", () => {
    // The label is Tamagui Text, so its weight arrives as an atomic `_fw-*` class on the span.
    const weight = (selected: boolean) =>
      renderToStaticMarkup(
        <LoomarrProvider>
          <Action density="tv" selected={selected} tone="secondary">
            Guide
          </Action>
        </LoomarrProvider>,
      ).match(/_fw-(\d+)[^"]*">Guide</)?.[1];
    expect(weight(true)).toBe("600");
    expect(weight(false)).toBe("400");
  });

  it("draws an unselected focused action with the same amber border", () => {
    act(() => {
      root.render(
        <LoomarrProvider>
          <Action density="tv" tone="secondary">
            5 s
          </Action>
        </LoomarrProvider>,
      );
    });
    expect(button().style.borderTopColor).not.toBe(amber);
    act(() => button().focus());
    expect(button().style.borderTopColor).toBe(amber);
  });
});

describe("Action primary focus (WCAG 2.4.7)", () => {
  let host: HTMLDivElement;
  let root: Root;
  const button = () => host.querySelector<HTMLElement>('[role="button"]') as HTMLElement;
  const look = (el: HTMLElement) => ({
    backgroundColor: el.style.backgroundColor,
    borderTopColor: el.style.borderTopColor,
    outlineColor: el.style.outlineColor,
    outlineOffset: el.style.outlineOffset,
    outlineStyle: el.style.outlineStyle,
    outlineWidth: el.style.outlineWidth,
  });
  // WCAG relative luminance and contrast ratio, from a "#RRGGBB" token.
  const luminance = (hex: string) => {
    const [r, g, b] = [1, 3, 5].map((i) => {
      const c = Number.parseInt(hex.slice(i, i + 2), 16) / 255;
      return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
    }) as [number, number, number];
    return 0.2126 * r + 0.7152 * g + 0.0722 * b;
  };
  const contrast = (a: string, b: string) => {
    const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x) as [number, number];
    return (hi + 0.05) / (lo + 0.05);
  };

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

  // The primary's fill is amber, so an amber focus border on it was invisible (the tvOS Retry
  // audit, shots 41/42). Focus now adds a foreground ring outside a gap of the surface behind it.
  // Every solid fill gets the ring: an amber border is invisible on amber and under 3:1 on red.
  const fills = [
    { name: "primary", props: { tone: "primary" } },
    { name: "danger", props: { tone: "danger" } },
    { name: "suggest", props: { variant: "suggest" } },
    { name: "destructive", props: { variant: "destructive" } },
  ] as const;
  it.each(
    (["pointer", "touch", "tv"] as const).flatMap((density) => fills.map((fill) => ({ density, ...fill }))),
  )("draws a focused $density $name fill differently from its rest look", ({ density, props }) => {
    act(() => {
      root.render(
        <LoomarrProvider>
          <Action density={density} {...props}>
            Retry
          </Action>
        </LoomarrProvider>,
      );
    });
    const rest = look(button());
    expect(rest.outlineStyle).not.toBe("solid");

    act(() => button().focus());
    const focusedLook = look(button());
    expect(focusedLook).not.toEqual(rest);
    expect(focusedLook.backgroundColor).toBe(rest.backgroundColor);
    expect(focusedLook.outlineStyle).toBe("solid");
    expect(focusedLook.outlineWidth).toBe("3px");
    expect(focusedLook.outlineColor).toBe("rgb(231, 234, 240)");
    expect(Number.parseFloat(focusedLook.outlineOffset)).toBeGreaterThan(0);

    act(() => button().blur());
    expect(look(button())).toEqual(rest);
  });

  it.each(["dark", "light"] as const)(
    "keeps the %s ring at 3:1 against the surface, and the gap at 3:1 against every fill",
    (mode) => {
      const theme = semanticThemes[mode];
      const fillColors = [theme.action.primary, theme.state.danger, theme.guide.onAir, brandChroma[4]];
      for (const surface of [theme.surface.canvas, theme.surface.raised, theme.surface.elevated]) {
        expect(contrast(theme.content.primary, surface)).toBeGreaterThanOrEqual(3);
        for (const fill of fillColors) expect(contrast(surface, fill)).toBeGreaterThanOrEqual(3);
      }
    },
  );

  it("keeps the amber focus border on a secondary action, with no ring", () => {
    act(() => {
      root.render(
        <LoomarrProvider>
          <Action tone="secondary">Cancel</Action>
        </LoomarrProvider>,
      );
    });
    act(() => button().focus());
    expect(button().style.borderTopColor).toBe("rgb(255, 176, 32)");
    expect(button().style.outlineStyle).not.toBe("solid");
  });
});

describe("Action behind a modal", () => {
  let host: HTMLDivElement;
  let root: Root;
  const amber = "rgb(255, 176, 32)";
  const byText = (text: string) =>
    [...host.querySelectorAll<HTMLElement>('[role="button"]')].find(
      (el) => el.textContent === text,
    ) as HTMLElement;

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

  // A native Modal never blurs the control that opened it, so its focus border showed through
  // the scrim. The opener drops its focus look while a modal layer above it is open.
  it("drops the opener's focus look while a modal is open, and keeps it inside the modal", () => {
    const render = (visible: boolean) =>
      act(() => {
        root.render(
          <LoomarrProvider>
            <Action tone="secondary">Open</Action>
            <ModalLayer visible={visible}>
              <Action tone="secondary">Inside</Action>
            </ModalLayer>
          </LoomarrProvider>,
        );
      });

    render(false);
    act(() => byText("Open").focus());
    expect(byText("Open").style.borderTopColor).toBe(amber);

    render(true);
    expect(byText("Open").style.borderTopColor).not.toBe(amber);
    act(() => byText("Inside").focus());
    expect(byText("Inside").style.borderTopColor).toBe(amber);

    render(false);
    act(() => byText("Open").focus());
    expect(byText("Open").style.borderTopColor).toBe(amber);
  });

  // Sibling modals are common (a confirm opened from a sheet), so opening order decides which
  // layer is in front, not tree depth.
  it("puts a control in the first modal behind a sibling modal opened over it", () => {
    const render = (second: boolean) =>
      act(() => {
        root.render(
          <LoomarrProvider>
            <ModalLayer visible>
              <Action tone="secondary">First</Action>
            </ModalLayer>
            <ModalLayer visible={second}>
              <Action tone="secondary">Second</Action>
            </ModalLayer>
          </LoomarrProvider>,
        );
      });

    render(false);
    act(() => byText("First").focus());
    expect(byText("First").style.borderTopColor).toBe(amber);

    render(true);
    expect(byText("First").style.borderTopColor).not.toBe(amber);

    render(false);
    expect(byText("First").style.borderTopColor).toBe(amber);
  });
});

describe("Toggle and ChoiceGroup checked (#1909)", () => {
  // The box is a Tamagui Surface, so its colours arrive as token CSS variables, not inline hex.
  const neutralPressed = (markup: string) => {
    expect(markup).toContain("background-color:var(--surfaceElevated)");
    expect(markup).toContain("border-top-color:var(--contentPrimary)");
    expect(markup).not.toContain("var(--actionPrimary)");
  };

  it.each(["checkbox", "switch"] as const)(
    "draws a checked %s neutral, never amber, and keeps aria-checked",
    (kind) => {
      const markup = renderToStaticMarkup(
        <LoomarrProvider>
          <Toggle checked kind={kind} label="Approve" onCheckedChange={() => undefined} />
        </LoomarrProvider>,
      );
      expect(markup).toContain('aria-checked="true"');
      neutralPressed(markup);
    },
  );

  it("draws the chosen radio neutral", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <ChoiceGroup
          label="Span"
          onValueChange={() => undefined}
          options={[
            { label: "2h", value: "2" },
            { label: "4h", value: "4" },
          ]}
          value="4"
        />
      </LoomarrProvider>,
    );
    expect(markup).toContain('aria-checked="true"');
    neutralPressed(markup);
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
