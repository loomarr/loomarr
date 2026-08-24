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

  it("returns transient browsing to playback before allowing the host to exit", () => {
    expect(clientBackDestination("guide")).toBe("watching");
    expect(clientBackDestination("surf")).toBe("watching");
    expect(clientBackDestination("watching")).toBeNull();
  });
});
