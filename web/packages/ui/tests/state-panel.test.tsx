import { LoomarrProvider } from "@loomarr/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { StatePanel } from "../index";

describe("StatePanel", () => {
  it("announces an error and exposes one explicit recovery action", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <StatePanel
          action={{ label: "Try again", onPress: vi.fn() }}
          description="The guide could not be refreshed."
          kind="error"
          title="Couldn’t load the guide"
        />
      </LoomarrProvider>,
    );

    expect(markup).toContain('role="alert"');
    expect(markup).toContain("Couldn’t load the guide");
    expect(markup).toContain("The guide could not be refreshed.");
    expect(markup.match(/Try again/g)).toHaveLength(1);
  });

  it("marks loading as a polite busy status without inventing an action", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider theme="light">
        <StatePanel kind="loading" title="Loading channels" />
      </LoomarrProvider>,
    );

    expect(markup).toContain('aria-busy="true"');
    expect(markup).toContain('aria-live="polite"');
    expect(markup).toContain("Loading channels");
    expect(markup).not.toContain("button");
  });
});
