import { LoomarrProvider } from "@loomarr/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { ClientNavigation, clientBackDestination } from "../index";

describe("shared client navigation", () => {
  it("publishes a labelled navigation region with one selected destination", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <ClientNavigation active="guide" onNavigate={vi.fn()} />
      </LoomarrProvider>,
    );

    expect(markup).toContain('role="navigation"');
    expect(markup).toContain('aria-label="Primary navigation"');
    expect(markup.match(/role="button"/g)).toHaveLength(3);
    expect(markup.match(/aria-pressed="true"/g)).toHaveLength(1);
  });

  it("gives a phone its platform's tab bar instead of the row of actions", () => {
    const render = (variant?: "material") =>
      renderToStaticMarkup(
        <LoomarrProvider>
          <ClientNavigation active="watching" density="touch" onNavigate={vi.fn()} variant={variant} />
        </LoomarrProvider>,
      );

    for (const markup of [render(), render("material")]) {
      expect(markup).toContain('role="tablist"');
      expect(markup).not.toContain('role="button"');
      expect(markup.match(/role="tab"/g)).toHaveLength(3);
      expect(markup.match(/aria-selected="true"/g)).toHaveLength(1);
    }
    // The web build of a touch surface reads as iPhone's bar; Android's is chosen by platform.
    expect(render()).toContain("font-size:10px");
    expect(render("material")).toContain("font-size:12px");
  });

  it("returns transient browsing to playback before allowing the host to exit", () => {
    expect(clientBackDestination("guide")).toBe("watching");
    expect(clientBackDestination("surf")).toBe("watching");
    expect(clientBackDestination("watching")).toBeNull();
  });
});
