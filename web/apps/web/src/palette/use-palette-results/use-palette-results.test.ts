import { describe, expect, it } from "vitest";
import { diagnosticPaletteResult } from "./use-palette-results";

describe("diagnosticPaletteResult", () => {
  it.each(["diagnostics", "logs", "health", "playout"])("finds admin diagnostics with %s", (query) => {
    expect(diagnosticPaletteResult(query, true)).toMatchObject({ scope: "diagnostics", name: "Diagnostics" });
  });

  it("is absent before a useful query and for members", () => {
    expect(diagnosticPaletteResult("", true)).toBeUndefined();
    expect(diagnosticPaletteResult("l", true)).toBeUndefined();
    expect(diagnosticPaletteResult("logs", false)).toBeUndefined();
  });
});
