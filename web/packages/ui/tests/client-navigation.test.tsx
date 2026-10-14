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
      expect(markup.match(/role="tab"/g)).toHaveLength(4);
      expect(markup.match(/aria-selected="true"/g)).toHaveLength(1);
    }
    // The web build of a touch surface reads as iPhone's bar; Android's is chosen by platform.
    expect(render()).toContain("font-size:10px");
    expect(render("material")).toContain("font-size:12px");
  });

  it("orders a phone's tabs Watching, Guide, Requests, Surf and badges only Requests", () => {
    const render = (requestsBadge?: number) =>
      renderToStaticMarkup(
        <LoomarrProvider>
          <ClientNavigation
            active="watching"
            density="touch"
            onNavigate={vi.fn()}
            requestsBadge={requestsBadge}
          />
        </LoomarrProvider>,
      );
    const labels = [...render(3).matchAll(/aria-label="([^"]+)"/g)].map(([, label]) => label);

    expect(labels.filter((label) => label !== "Primary navigation")).toEqual([
      "Watching",
      "Guide",
      "Requests, 3 need attention",
      "Surf",
    ]);
    expect(render(0)).not.toContain("need attention");
    expect(render()).not.toContain("need attention");
  });

  it("keeps Requests off the TV and pointer rows", () => {
    for (const density of ["tv", "pointer"] as const) {
      const markup = renderToStaticMarkup(
        <LoomarrProvider>
          <ClientNavigation active="watching" density={density} onNavigate={vi.fn()} requestsBadge={5} />
        </LoomarrProvider>,
      );
      expect(markup).not.toContain("Requests");
      expect(markup.match(/role="button"/g)).toHaveLength(3);
    }
  });

  it("returns transient browsing to playback before allowing the host to exit", () => {
    expect(clientBackDestination("requests")).toBe("watching");
    expect(clientBackDestination("guide")).toBe("watching");
    expect(clientBackDestination("surf")).toBe("watching");
    expect(clientBackDestination("watching")).toBeNull();
  });
});
