// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";

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

  it("is not a button without onPress", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <StatusLabel tone="warn">Overriding LOOMARR_PORT</StatusLabel>
      </LoomarrProvider>,
    );
    expect(markup).not.toContain('role="button"');
  });
});

describe("StatusLabel with onPress", () => {
  afterEach(cleanup);

  it("is a named button that runs onPress by click and by keyboard", async () => {
    const onPress = vi.fn();
    render(
      <LoomarrProvider>
        <StatusLabel aria-label="Hand Port back" onPress={onPress} tone="warn">
          Overriding LOOMARR_PORT
        </StatusLabel>
      </LoomarrProvider>,
    );

    const button = screen.getByRole("button", { name: "Hand Port back" });
    expect(button.textContent).toContain("Overriding LOOMARR_PORT");
    await userEvent.click(button);
    expect(onPress).toHaveBeenCalledOnce();

    button.focus();
    await userEvent.keyboard("{Enter}");
    expect(onPress).toHaveBeenCalledTimes(2);
  });
});
