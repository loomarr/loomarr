import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { LoomarrProvider, ProgressTrack } from "../index";

describe("shared primitives", () => {
  it("names determinate progress for assistive technology", () => {
    const output = renderToStaticMarkup(
      <LoomarrProvider>
        <ProgressTrack accessibilityLabel="The Current Frontier" percent={42} />
      </LoomarrProvider>,
    );

    expect(output).toContain('role="progressbar"');
    expect(output).toContain('aria-label="The Current Frontier"');
    expect(output).toContain('aria-valuenow="42"');
  });
});
