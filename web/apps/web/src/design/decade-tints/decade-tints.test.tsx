import { LoomarrProvider, semanticThemes } from "@loomarr/design-system";
import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { DecadeTints } from "./decade-tints";

describe("DecadeTints", () => {
  it.each([
    ["dark", "Dark theme"],
    ["light", "Light theme"],
  ] as const)("shows the %s set with its neutral", (mode, title) => {
    render(
      <LoomarrProvider theme="dark">
        <DecadeTints />
      </LoomarrProvider>,
    );

    const panel = within(screen.getByRole("region", { name: title }));
    for (const hex of Object.values(semanticThemes[mode].decadeTint)) {
      expect(panel.getByText(hex)).toBeTruthy();
    }
  });
});
