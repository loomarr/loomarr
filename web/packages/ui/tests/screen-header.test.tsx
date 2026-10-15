import { LoomarrProvider } from "@loomarr/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { ScreenHeader } from "../index";

const render = (node: React.ReactNode) =>
  renderToStaticMarkup(<LoomarrProvider theme="dark">{node}</LoomarrProvider>);
const buttons = (markup: string) => [...markup.matchAll(/aria-label="([^"]+)"/g)].map(([, label]) => label);

describe("ScreenHeader", () => {
  const plus = { accessibilityLabel: "Request a channel", icon: "plus", onPress: vi.fn() } as const;
  const account = { accessibilityLabel: "Device and account", icon: "account", onPress: vi.fn() } as const;

  it("draws no button without actions", () => {
    expect(buttons(render(<ScreenHeader title="Guide" />))).toEqual([]);
  });

  it("draws one action", () => {
    expect(buttons(render(<ScreenHeader actions={[account]} title="Guide" />))).toEqual([
      "Device and account",
    ]);
  });

  it("draws two actions, in reading order, the first one keeping Requests' plus", () => {
    const markup = render(<ScreenHeader actions={[plus, account]} title="Requests" />);
    expect(buttons(markup)).toEqual(["Request a channel", "Device and account"]);
    expect(markup).toContain("Requests");
  });
});
