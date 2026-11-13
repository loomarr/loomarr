// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { InlineAction, LoomarrProvider } from "../index";

afterEach(cleanup);

const inSentence = (props: Partial<React.ComponentProps<typeof InlineAction>> = {}) => (
  <LoomarrProvider theme="dark">
    <p>
      Following this installation (US).{" "}
      <InlineAction onPress={() => undefined} {...props}>
        Use another place
      </InlineAction>
    </p>
  </LoomarrProvider>
);

describe("InlineAction", () => {
  it("is a button inside the sentence, drawn as the sentence's own text with an underline", () => {
    render(inSentence());

    const button = screen.getByRole("button", { name: "Use another place" });
    expect(button.parentElement?.tagName).toBe("P");
    expect(button.getAttribute("type")).toBe("button");
    const style = getComputedStyle(button);
    // Inline flow, the surrounding type, no box.
    expect(style.display).toBe("inline");
    expect(button.style.font).toBe("inherit");
    expect(style.textDecorationLine).toBe("underline");
    expect(style.borderWidth).toMatch(/^0/);
    // A touch screen gets the 24 px tap area.
    expect(button.hasAttribute("data-touch-target")).toBe(true);
  });

  it("presses with the mouse and with Enter and Space", async () => {
    const onPress = vi.fn();
    const user = userEvent.setup();
    render(inSentence({ onPress }));

    await user.click(screen.getByRole("button"));
    await user.keyboard("{Enter}");
    await user.keyboard(" ");

    expect(onPress).toHaveBeenCalledTimes(3);
  });

  it("takes an accessible name that says more than the label", () => {
    render(inSentence({ "aria-label": "Use another place for this channel" }));

    expect(screen.getByRole("button", { name: "Use another place for this channel" })).toBeTruthy();
  });

  it("draws the focus outline only while focused", async () => {
    const user = userEvent.setup();
    render(inSentence());
    const button = screen.getByRole("button");
    expect(button.style.outline).toBe("none");

    await user.tab();

    expect(document.activeElement).toBe(button);
    expect(button.style.outline).toContain("2px solid");
  });

  it("does not press when disabled, and reads as unavailable", async () => {
    const onPress = vi.fn();
    const user = userEvent.setup();
    render(inSentence({ disabled: true, onPress }));
    const button = screen.getByRole("button") as HTMLButtonElement;

    await user.click(button);

    expect(onPress).not.toHaveBeenCalled();
    expect(button.disabled).toBe(true);
    expect(button.style.opacity).toBe("0.5");
  });
});
