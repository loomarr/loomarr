// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  Disclosure,
  LoomarrProvider,
  resolveAdaptiveDirection,
  ScrollFrame,
  SectionHeader,
  Text,
} from "../index";

describe("shared adaptive layout", () => {
  it("resolves narrow and wide composition from density-specific breakpoints", () => {
    expect(resolveAdaptiveDirection(899, "pointer")).toBe("column");
    expect(resolveAdaptiveDirection(900, "pointer")).toBe("row");
    expect(resolveAdaptiveDirection(1279, "tv")).toBe("column");
    expect(resolveAdaptiveDirection(1280, "tv")).toBe("row");
    expect(resolveAdaptiveDirection(600, "touch", 580)).toBe("row");
  });

  it("publishes disclosure state and points its control at the body", () => {
    const collapsed = renderToStaticMarkup(
      <LoomarrProvider>
        <Disclosure expanded={false} label="Episode information" onExpandedChange={vi.fn()}>
          Classic episode details
        </Disclosure>
      </LoomarrProvider>,
    );
    const expanded = renderToStaticMarkup(
      <LoomarrProvider theme="light">
        <Disclosure expanded label="Episode information" onExpandedChange={vi.fn()}>
          Classic episode details
        </Disclosure>
      </LoomarrProvider>,
    );

    expect(collapsed).toContain('aria-expanded="false"');
    // On web a collapsed body stays mounted (hidden until found once mounted; see below), so the
    // control names an element in both states.
    const collapsedControls = collapsed.match(/aria-controls="([^"]+)"/);
    expect(collapsed).toContain(`id="${collapsedControls?.[1]}"`);
    expect(expanded).toContain('aria-expanded="true"');
    expect(expanded).toContain("Classic episode details");
    const controls = expanded.match(/aria-controls="([^"]+)"/);
    expect(controls).not.toBeNull();
    expect(expanded).toContain(`id="${controls?.[1]}"`);
  });

  it("keeps scroll content behind the package interface", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <ScrollFrame>
          <Text textRole="body">Scrollable programme detail</Text>
        </ScrollFrame>
      </LoomarrProvider>,
    );

    expect(markup).toContain("Scrollable programme detail");
    expect(markup).toContain("flex-grow:1");
  });

  it("titles a section with a labelled heading, its meta and trailing controls", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <SectionHeader id="home-watching" meta="3 people" title="Watching now">
          <a href="/guide">Full guide</a>
        </SectionHeader>
      </LoomarrProvider>,
    );

    expect(markup).toMatch(/<h2[^>]*id="home-watching"[^>]*>Watching now<\/h2>/);
    expect(markup).toContain("3 people");
    expect(markup).toContain('<a href="/guide">Full guide</a>');
  });
});

describe("Disclosure (web)", () => {
  afterEach(cleanup);

  const body = () => screen.getByText("Classic episode details").closest("[id^='loomarr-disclosure-']");
  const toggle = () => screen.getByRole("button", { name: /Episode information/ });
  const state = () => toggle().getAttribute("aria-expanded");

  it("opens and closes on its own when no state is given", async () => {
    render(
      <LoomarrProvider>
        <Disclosure label="Episode information">Classic episode details</Disclosure>
      </LoomarrProvider>,
    );
    expect(state()).toBe("false");
    await userEvent.click(toggle());
    expect(state()).toBe("true");
    await userEvent.click(toggle());
    expect(state()).toBe("false");
  });

  it("starts open from defaultExpanded", () => {
    render(
      <LoomarrProvider>
        <Disclosure defaultExpanded label="Episode information">
          Classic episode details
        </Disclosure>
      </LoomarrProvider>,
    );

    expect(state()).toBe("true");
    expect(body()?.hasAttribute("hidden")).toBe(false);
  });

  it("follows a controlling parent and only asks it to change", async () => {
    const onExpandedChange = vi.fn();
    render(
      <LoomarrProvider>
        <Disclosure expanded={false} label="Episode information" onExpandedChange={onExpandedChange}>
          Classic episode details
        </Disclosure>
      </LoomarrProvider>,
    );
    await userEvent.click(toggle());

    expect(onExpandedChange).toHaveBeenCalledWith(true);
    expect(state()).toBe("false");
  });

  // Find-in-page: a closed body is still mounted, hidden until the browser finds text in it.
  it("keeps a closed body mounted and hidden until found", () => {
    render(
      <LoomarrProvider>
        <Disclosure label="Episode information">Classic episode details</Disclosure>
      </LoomarrProvider>,
    );

    expect(body()?.getAttribute("hidden")).toBe("until-found");
  });

  it("opens when find-in-page matches text inside it", async () => {
    const onExpandedChange = vi.fn();
    render(
      <LoomarrProvider>
        <Disclosure label="Episode information" onExpandedChange={onExpandedChange}>
          Classic episode details
        </Disclosure>
      </LoomarrProvider>,
    );

    body()?.dispatchEvent(new Event("beforematch"));

    await waitFor(() => expect(state()).toBe("true"));
    expect(body()?.hasAttribute("hidden")).toBe(false);
    expect(onExpandedChange).toHaveBeenCalledWith(true);
  });

  it("draws a status dot before the label, its words in the description", () => {
    render(
      <LoomarrProvider>
        <Disclosure description="needs attention" label="Emby" status="error" statusLabel="Failing">
          Fix the address
        </Disclosure>
      </LoomarrProvider>,
    );

    expect(screen.getByRole("img", { name: "Failing" })).toBeTruthy();
    expect(screen.getByText("needs attention")).toBeTruthy();
  });
});
