// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { Action, LoomarrProvider, Tooltip } from "../index";

// The web Tooltip, on Base UI. It owes a browser user the label on hover and on keyboard focus,
// and it must leave the trigger's own name and handlers alone. The native adapter renders the
// trigger only and is not exercised here.

afterEach(cleanup);

const renderTooltip = (ui: React.ReactNode) => render(<LoomarrProvider>{ui}</LoomarrProvider>);

describe("Tooltip (web)", () => {
  it("says what the control does on hover, and the control keeps its own name", async () => {
    renderTooltip(
      <Tooltip content="Remove from lineup">
        <button type="button" aria-label="Remove Bluey">
          x
        </button>
      </Tooltip>,
    );

    await userEvent.hover(screen.getByRole("button", { name: "Remove Bluey" }));
    expect((await screen.findByRole("tooltip")).textContent).toBe("Remove from lineup");
  });

  it("opens from keyboard focus", async () => {
    renderTooltip(
      <Tooltip content="Sign out">
        <button type="button" aria-label="Sign out">
          x
        </button>
      </Tooltip>,
    );

    await userEvent.tab();
    expect((await screen.findByRole("tooltip")).textContent).toBe("Sign out");
  });

  it("carries the id a trigger describes itself by", async () => {
    renderTooltip(
      <Tooltip content="Cereal ad, 0:30" id="segment-tip">
        <button type="button" aria-describedby="segment-tip">
          Cereal ad
        </button>
      </Tooltip>,
    );

    await userEvent.hover(screen.getByRole("button", { name: "Cereal ad" }));
    expect((await screen.findByRole("tooltip")).getAttribute("id")).toBe("segment-tip");
  });

  // An Action is the trigger the web app's buttons migrate to, so the Base UI trigger has to land
  // its hover and focus handlers on the Pressable's host element, not lose them in the bridge.
  it("opens over an Action on hover", async () => {
    renderTooltip(
      <Tooltip content="Go further back in time">
        <Action variant="ghost">Older</Action>
      </Tooltip>,
    );

    await userEvent.hover(screen.getByRole("button", { name: "Older" }));
    expect((await screen.findByRole("tooltip")).textContent).toBe("Go further back in time");
  });

  it("opens over an Action from keyboard focus", async () => {
    renderTooltip(
      <Tooltip content="Go further back in time">
        <Action variant="ghost">Older</Action>
      </Tooltip>,
    );

    await userEvent.tab();
    expect((await screen.findByRole("tooltip")).textContent).toBe("Go further back in time");
  });
});
