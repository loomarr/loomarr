import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { LoomarrProvider, StatusLabel } from "../index";
import { withoutRuntimeMarker } from "./support/atomic-css";

describe("StatusLabel", () => {
  it("is a dot and a sentence-case word, with no filled chip", () => {
    const markup = withoutRuntimeMarker(
      renderToStaticMarkup(
        <LoomarrProvider>
          <StatusLabel tone="ok">Enabled</StatusLabel>
        </LoomarrProvider>,
      ),
    );
    expect(markup).toContain("Enabled");
    expect(markup).toContain("aria-hidden");
    expect(markup).not.toContain('role="img"');
    expect(markup).not.toMatch(/text-transform:\s*uppercase|uppercase/);
  });
});
