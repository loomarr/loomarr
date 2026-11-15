import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { LoomarrProvider, Stack, semanticThemes } from "../index";
import { washes } from "../src/tokens";

const render = (mode: "dark" | "light", node: React.ReactNode) =>
  renderToStaticMarkup(<LoomarrProvider theme={mode}>{node}</LoomarrProvider>);

// The wash is the base stop at the alpha the app's `bg-<colour>/<n>` utility drew.
const expected = {
  washCanvas60: "rgba(11,12,14,0.6)",
  washElevated20: "rgba(27,30,36,0.2)",
  washElevated30: "rgba(27,30,36,0.3)",
  washElevated35: "rgba(27,30,36,0.35)",
  washElevated40: "rgba(27,30,36,0.4)",
  washElevated50: "rgba(27,30,36,0.5)",
  washOnair10: "rgba(229,72,77,0.1)",
  washOnair5: "rgba(229,72,77,0.05)",
  washRaised80: "rgba(19,21,25,0.8)",
  washSignal10: "rgba(255,176,32,0.1)",
  washSignal28: "rgba(255,176,32,0.28)",
  washSignal90: "rgba(255,176,32,0.9)",
  washTune5: "rgba(76,201,232,0.05)",
} as const;

describe("alpha washes", () => {
  it("draws each base stop at its alpha", () => {
    expect(washes).toEqual(expected);
  });

  it("builds the surface washes from the dark palette's own stops", () => {
    expect(semanticThemes.dark.surface.elevated).toBe("#1B1E24");
    expect(semanticThemes.dark.surface.canvas).toBe("#0B0C0E");
    expect(semanticThemes.dark.surface.raised).toBe("#131519");
  });

  it.each(["dark", "light"] as const)("is the same wash in the %s theme", (mode) => {
    const markup = render(
      mode,
      <Stack backgroundColor="washElevated40">
        <span>x</span>
      </Stack>,
    );

    expect(markup).toContain("background-color:var(--washElevated40)");
    // The theme's own variable holds the rgba, so the value is the same under either theme.
    expect(markup).toContain(expected.washElevated40);
  });
});
