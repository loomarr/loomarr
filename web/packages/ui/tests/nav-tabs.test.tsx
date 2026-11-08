// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import { type LinkLike, NavTabs } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// This package has no DOM lib (it targets React Native too), so the slice of the jsdom document
// the test touches is typed here.
type Style = Record<string, string>;
type Node = {
  getAttribute(name: string): string | null;
  hasAttribute(name: string): boolean;
  querySelector(s: string): Node | null;
  querySelectorAll(s: string): ArrayLike<Node>;
  style: Style;
  textContent: string | null;
};
type Doc = {
  body: Node & { append(node: unknown): void; replaceChildren(): void };
  createElement(tag: string): Parameters<typeof createRoot>[0];
};
const doc = (globalThis as unknown as { document: Doc }).document;
const elementPrototype = (globalThis as unknown as { HTMLElement: { prototype: object } }).HTMLElement
  .prototype;

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
  vi.restoreAllMocks();
});

// A plain anchor stand-in: NavTabs takes the router's Link injected so it stays router-agnostic.
const anchorLink: LinkLike = ({ activeOptions: _activeOptions, children, search: _search, to, ...rest }) => (
  <a href={to} {...(rest as Record<string, unknown>)}>
    {children}
  </a>
);

const tabs = [
  { id: "approval", label: "Needs approval", to: "/queue/approval", count: 3 },
  { id: "flight", label: "In flight", to: "/queue/flight", count: 12 },
  { id: "history", label: "History", to: "/queue/history", count: 14 },
];

const nav = () => doc.body.querySelector("nav") as Node;
const link = (href: string) => doc.body.querySelector(`a[href="${href}"]`) as Node;
const count = (href: string) => link(href).querySelector("span") as Node;

describe("NavTabs (web)", () => {
  // #1785: at phone width the bar stays one line and scrolls sideways; wrapping put a channel's
  // Danger zone, or Requests' Done, on a second row.
  it("keeps the destinations on one line that scrolls sideways", async () => {
    await mount(<NavTabs activeId="flight" label="Queue sections" linkComponent={anchorLink} tabs={tabs} />);

    expect(nav().getAttribute("aria-label")).toBe("Queue sections");
    expect(nav().style.overflowX).toBe("auto");
    expect(nav().style.flexShrink).toBe("0");
    expect(nav().style.flexWrap).toBe("");
  });

  it("scrolls the active destination into view, sideways only", async () => {
    // jsdom has no layout: a 200 px bar whose third tab sits at 260–340 px of its content.
    let scrollLeft = 0;
    const rect = (left: number, width: number) => ({ left, width });
    vi.spyOn(
      elementPrototype as { getBoundingClientRect(): unknown },
      "getBoundingClientRect",
    ).mockReturnValue(rect(0, 200));
    vi.spyOn(elementPrototype as { clientWidth: number }, "clientWidth", "get").mockReturnValue(200);
    const scroll = vi
      .spyOn(elementPrototype as { scrollLeft: number }, "scrollLeft", "set")
      .mockImplementation((value: number) => {
        scrollLeft = value;
      });
    vi.spyOn(elementPrototype as { scrollLeft: number }, "scrollLeft", "get").mockImplementation(
      () => scrollLeft,
    );
    const measuredLink: LinkLike = ({ activeOptions: _a, children, search: _s, to, ...rest }) => (
      <a
        href={to}
        ref={(node) => {
          if (node && to === "/queue/history")
            (node as unknown as { getBoundingClientRect(): unknown }).getBoundingClientRect = () =>
              rect(260 - scrollLeft, 80);
        }}
        {...(rest as Record<string, unknown>)}
      >
        {children}
      </a>
    );

    await mount(
      <NavTabs activeId="history" label="Queue sections" linkComponent={measuredLink} tabs={tabs} />,
    );

    expect(scroll).toHaveBeenCalledWith(140);
  });

  it("renders a real link per tab with its count", async () => {
    await mount(<NavTabs activeId="flight" label="Queue sections" linkComponent={anchorLink} tabs={tabs} />);

    expect(doc.body.querySelectorAll("a").length).toBe(3);
    expect(link("/queue/approval").textContent).toBe("Needs approval3");
    expect(link("/queue/history").textContent).toBe("History14");
  });

  // ⚠ Omitted, not "0": absence means counting does not apply to this tab, and "0" means zero
  // things are here. A tab with no count carries none.
  it("omits the count pill entirely when a tab has none", async () => {
    await mount(
      <NavTabs
        activeId="sources"
        label="Filler sections"
        linkComponent={anchorLink}
        tabs={[{ id: "sources", label: "Sources", to: "/filler/sources" }]}
      />,
    );

    expect(link("/filler/sources").textContent).toBe("Sources");
    expect(link("/filler/sources").querySelector("span")).toBeNull();
  });

  // These are NAVIGATION, not the ARIA tablist pattern: the active destination is marked with
  // `aria-current`, and that is the WHOLE vocabulary. `aria-controls` would point at a panel that
  // may not be mounted, which is axe's `aria-valid-attr-value` at CRITICAL impact.
  it("marks the active destination with aria-current and no aria-controls", async () => {
    await mount(<NavTabs activeId="history" label="Queue sections" linkComponent={anchorLink} tabs={tabs} />);

    expect(link("/queue/history").getAttribute("aria-current")).toBe("page");
    expect(link("/queue/history").hasAttribute("aria-controls")).toBe(false);
    expect(link("/queue/flight").hasAttribute("aria-current")).toBe(false);
    expect(link("/queue/flight").hasAttribute("aria-controls")).toBe(false);
  });

  it("draws the active pill on the elevated fill with primary text, never amber (#1909 Q1)", async () => {
    await mount(<NavTabs activeId="flight" label="Queue sections" linkComponent={anchorLink} tabs={tabs} />);

    const active = link("/queue/flight").style;
    expect(active.backgroundColor).toBe("var(--surfaceElevated)");
    expect(active.color).toBe("var(--contentPrimary)");
    expect(active.fontWeight).toBe("500");
    expect(link("/queue/approval").style.backgroundColor).toBe("transparent");
    expect(nav().style.paddingBottom).toBe("8px");
  });

  // #1659 web mock: a pill bar tints waiting work too, and keeps the tint on the active tab.
  it("tints an attention count on the pill bar, even while that tab is active", async () => {
    await mount(
      <NavTabs
        activeId="needs"
        label="Requests sections"
        linkComponent={anchorLink}
        tabs={[
          { attention: true, count: 3, id: "needs", label: "Needs you", to: "/requests/needs-you" },
          { count: 9, id: "done", label: "Done", to: "/requests/done" },
        ]}
      />,
    );

    expect(count("/requests/needs-you").style.color).toBe("var(--accentSuggest)");
    expect(count("/requests/done").style.backgroundColor).toBe("var(--surfaceElevated)");
  });

  // The v2 mock's Queue tab bar: transparent tabs, a 2px bottom border in the active colour, and
  // a 1px rule under the whole bar.
  describe("underline variant", () => {
    const mountUnderline = () =>
      mount(
        <NavTabs
          activeId="needs"
          label="Requests sections"
          linkComponent={anchorLink}
          tabs={[
            { attention: true, count: 2, id: "needs", label: "Needs you", to: "/requests/needs-you" },
            { count: 0, id: "progress", label: "In progress", to: "/requests/in-progress" },
          ]}
          variant="underline"
        />,
      );

    it("draws the active tab with a 2px primary underline and no pill fill", async () => {
      await mountUnderline();

      const active = link("/requests/needs-you").style;
      expect(active.borderBottom).toBe("2px solid var(--contentPrimary)");
      expect(active.backgroundColor).toBe("transparent");
      expect(active.borderRadius).toBe("");
      expect(link("/requests/in-progress").style.borderBottom).toBe("2px solid transparent");
    });

    it("rules the bar with a 1px bottom border, not the pill bar's padded one", async () => {
      await mountUnderline();

      expect(nav().style.borderBottom).toBe("1px solid var(--borderDecorative)");
      expect(nav().style.paddingBottom).toBe("0px");
    });

    it("tints the count of a tab that needs attention", async () => {
      await mountUnderline();

      expect(count("/requests/needs-you").style.color).toBe("var(--accentSuggest)");
      expect(count("/requests/in-progress").style.color).not.toBe("var(--accentSuggest)");
    });
  });

  it("passes search params through to the link", async () => {
    const searchLink: LinkLike = ({ children, search, to }) => (
      <a data-search={JSON.stringify(search)} href={to}>
        {children}
      </a>
    );
    await mount(
      <NavTabs
        activeId="catalog"
        label="Filler sections"
        linkComponent={searchLink}
        tabs={[{ id: "catalog", label: "Catalog", search: { view: "list" }, to: "/filler" }]}
      />,
    );

    expect(link("/filler").getAttribute("data-search")).toBe(JSON.stringify({ view: "list" }));
  });
});
