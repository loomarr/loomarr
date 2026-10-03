import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { BottomSheet, LoomarrProvider, Screen, TabBar, Text } from "../index";

const render = (node: React.ReactNode) =>
  renderToStaticMarkup(
    <LoomarrProvider insets={{ bottom: 34, left: 0, right: 0, top: 47 }} theme="dark">
      {node}
    </LoomarrProvider>,
  );

const items = [
  { icon: "play", label: "Watching", value: "watching" },
  { icon: "guide", label: "Guide", value: "guide" },
  { icon: "channels", label: "Surf", value: "surf" },
] as const;

// The phone chrome the native mock draws (#1659 5d-5g): the two platform tab bars and the sheet.
describe("phone chrome", () => {
  it("names one selected tab in a labelled tab list, in either idiom", () => {
    for (const idiom of ["ios", "material"] as const) {
      const markup = render(
        <TabBar
          accessibilityLabel="Primary navigation"
          idiom={idiom}
          items={items}
          onSelect={vi.fn()}
          selected="guide"
        />,
      );
      expect(markup).toContain('role="tablist"');
      expect(markup).toContain('aria-label="Primary navigation"');
      expect(markup.match(/role="tab"/g)).toHaveLength(3);
      expect(markup.match(/aria-selected="true"/g)).toHaveLength(1);
      for (const { label } of items) expect(markup).toContain(`>${label}<`);
    }
  });

  it("sizes each bar as measured: iPhone's 10 pt labels, Android's 12, both over the home inset", () => {
    const ios = render(
      <TabBar accessibilityLabel="Tabs" idiom="ios" items={items} onSelect={vi.fn()} selected="guide" />,
    );
    const material = render(
      <TabBar accessibilityLabel="Tabs" idiom="material" items={items} onSelect={vi.fn()} selected="guide" />,
    );
    expect(ios).toContain("font-size:10px");
    expect(material).toContain("font-size:12px");
    expect(ios).toContain("padding-bottom:34px");
    expect(material).toContain("padding-bottom:34px");
    expect(ios).toContain("min-height:49px");
    expect(material).toContain("min-height:80px");
  });

  it("counts what needs the person over a tab's glyph, and says so to a screen reader", () => {
    const withBadge = [
      ...items,
      { badge: 3, icon: "requests", label: "Requests", value: "requests" },
    ] as const;
    const markup = render(
      <TabBar accessibilityLabel="Tabs" idiom="ios" items={withBadge} onSelect={vi.fn()} selected="guide" />,
    );
    expect(markup.match(/role="tab"/g)).toHaveLength(4);
    expect(markup).toContain(">3<");
    expect(markup).toContain('aria-label="Requests, 3 need attention"');
    // Zero draws nothing, and a long count is capped.
    const none = [{ badge: 0, icon: "requests", label: "Requests", value: "requests" }] as const;
    expect(
      render(
        <TabBar accessibilityLabel="Tabs" idiom="ios" items={none} onSelect={vi.fn()} selected="requests" />,
      ),
    ).not.toContain("need attention");
    const many = [{ badge: 120, icon: "requests", label: "Requests", value: "requests" }] as const;
    expect(
      render(
        <TabBar accessibilityLabel="Tabs" idiom="ios" items={many} onSelect={vi.fn()} selected="requests" />,
      ),
    ).toContain(">99+<");
  });

  it("docks a footer under the content: no bottom gutter above it, the bar carries the inset", () => {
    const markup = render(
      <Screen density="touch" footer={<Text textRole="metadata">bar</Text>}>
        <Text textRole="metadata">content</Text>
      </Screen>,
    );
    expect(markup.indexOf("content")).toBeLessThan(markup.indexOf("bar"));
    expect(markup).not.toContain("padding-bottom:58px");
    expect(render(<Screen density="touch" />)).toContain("padding-bottom:58px");
  });

  it("draws the sheet's grabber only when asked, and never announces it", () => {
    const sheet = render(
      <BottomSheet onDismiss={vi.fn()}>
        <Text textRole="metadata">Watch now</Text>
      </BottomSheet>,
    );
    const strip = render(
      <BottomSheet handle={false}>
        <Text textRole="metadata">Watch</Text>
      </BottomSheet>,
    );
    expect(sheet).toContain('aria-hidden="true"');
    expect(strip).not.toContain('aria-hidden="true"');
  });
});
