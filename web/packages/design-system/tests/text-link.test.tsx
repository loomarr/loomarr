// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { LoomarrProvider, semanticThemes, TextLink } from "../index";

afterEach(cleanup);

const inTheme = (ui: React.ReactNode, theme: "dark" | "light" = "dark") => (
  <LoomarrProvider theme={theme}>{ui}</LoomarrProvider>
);

describe("TextLink", () => {
  it("is an anchor to the address, drawn in the info colour with an underline", () => {
    render(inTheme(<TextLink href="/help">Read the guide</TextLink>));

    const link = screen.getByRole("link", { name: "Read the guide" });
    expect(link.tagName).toBe("A");
    expect(link.getAttribute("href")).toBe("/help");
    expect(link.style.textDecorationLine).toBe("underline");
    expect(link.style.display).toBe("inline");
    // The same colour "Open on Archive.org" has always been: the theme's info stop.
    expect(link.style.color).not.toBe("");
    expect(link.hasAttribute("target")).toBe(false);
  });

  it("takes the info colour of each theme", () => {
    const dark = render(inTheme(<TextLink href="/a">One</TextLink>, "dark"));
    const darkColour = (dark.getByRole("link") as HTMLElement).style.color;
    dark.unmount();
    const light = render(inTheme(<TextLink href="/a">One</TextLink>, "light"));
    const lightColour = (light.getByRole("link") as HTMLElement).style.color;

    expect(darkColour).not.toBe(lightColour);
    expect(semanticThemes.dark.state.info).toBe("#4CC9E8");
    expect(semanticThemes.light.state.info).toBe("#08657A");
  });

  it("opens an external link in a new tab without a referrer, and says it leaves", () => {
    render(
      inTheme(
        <TextLink external href="https://archive.example/details/reel-12">
          Open on Archive.org
        </TextLink>,
      ),
    );

    const link = screen.getByRole("link", { name: "Open on Archive.org" });
    expect(link.getAttribute("target")).toBe("_blank");
    expect(link.getAttribute("rel")).toBe("noreferrer");
    // The way-out glyph sits after the label and is not part of the name.
    expect(link.querySelector("svg")).not.toBeNull();
    expect(link.lastElementChild?.tagName.toLowerCase()).toBe("svg");
    expect(link.style.display).toBe("inline-flex");
  });

  it("leads with its glyph when asked", () => {
    render(
      inTheme(
        <TextLink href="/filler" icon="goBack" iconPlacement="start">
          Back to filler
        </TextLink>,
      ),
    );

    expect(
      screen.getByRole("link", { name: "Back to filler" }).firstElementChild?.tagName.toLowerCase(),
    ).toBe("svg");
  });

  it("is drawn onto a router link, keeping its address, props and handlers", async () => {
    const onClick = vi.fn((event: { preventDefault: () => void }) => event.preventDefault());
    const user = userEvent.setup();
    render(
      inTheme(
        <TextLink
          icon="goTo"
          render={<a data-testid="route" href="/help?page=troubleshooting" onClick={onClick} />}
        >
          Fix
        </TextLink>,
      ),
    );

    const link = screen.getByTestId("route");
    expect(link.getAttribute("href")).toBe("/help?page=troubleshooting");
    expect(link.textContent).toBe("Fix");
    expect(link.style.textDecorationLine).toBe("underline");
    await user.click(link);
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it("is a button drawn as a link when it has a press and no address", async () => {
    const onPress = vi.fn();
    const user = userEvent.setup();
    render(inTheme(<TextLink onPress={onPress}>the approval gate</TextLink>));

    const button = screen.getByRole("button", { name: "the approval gate" });
    expect(button.getAttribute("type")).toBe("button");
    expect(button.style.textDecorationLine).toBe("underline");
    expect(button.style.borderWidth).toMatch(/^0/);
    await user.click(button);
    await user.keyboard("{Enter}");
    expect(onPress).toHaveBeenCalledTimes(2);
  });

  it("draws the focus outline only while focused", async () => {
    const user = userEvent.setup();
    render(inTheme(<TextLink href="/help">Read the guide</TextLink>));
    const link = screen.getByRole("link");
    expect(link.style.outline).toBe("none");

    await user.tab();

    expect(document.activeElement).toBe(link);
    expect(link.style.outline).toContain("2px solid");
  });

  it("takes an accessible name that says more than the label", () => {
    render(
      inTheme(
        <TextLink
          aria-label="Open the Archive.org page for Cereal ad, 1989"
          external
          href="https://archive.example/x"
        >
          View original
        </TextLink>,
      ),
    );

    expect(screen.getByRole("link", { name: "Open the Archive.org page for Cereal ad, 1989" })).toBeTruthy();
  });
});
