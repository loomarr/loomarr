// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";

import { LoomarrProvider, Notice } from "../index";
import { declarationsOf } from "./support/atomic-css";

afterEach(cleanup);

const renderNotice = (ui: React.ReactNode) => render(<LoomarrProvider>{ui}</LoomarrProvider>);

describe("Notice", () => {
  it("is a polite status that names what happened and what is true now", () => {
    renderNotice(<Notice body="Showing what Loomarr had a moment ago." title="Couldn't refresh the Guide" />);

    const status = screen.getByRole("status");
    expect(status.textContent).toContain("Couldn't refresh the Guide");
    expect(status.textContent).toContain("Showing what Loomarr had a moment ago.");
    expect(status.getAttribute("aria-live")).toBe("polite");
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("hides its tint dot from assistive tech, so the words carry the meaning", () => {
    renderNotice(<Notice body="Body" title="Title" tone="warn" />);

    expect(screen.queryByRole("img")).toBeNull();
    expect(screen.getByRole("status").querySelector('[aria-hidden="true"]')).not.toBeNull();
  });

  it("draws the warn tone with a warning outline and the neutral tone without one", () => {
    const outline = (tone?: "warn") => {
      const markup = renderToStaticMarkup(
        <LoomarrProvider>
          <Notice body="Body" title="Title" tone={tone} />
        </LoomarrProvider>,
      );
      const tag = /<[a-z]+[^>]*role="status"[^>]*>/.exec(markup)?.[0] ?? "";
      return declarationsOf(markup, tag);
    };

    expect(outline("warn")).toContain("stateWarning");
    expect(outline()).not.toContain("stateWarning");
  });

  it("outlines success and danger in their own state colours", () => {
    const outline = (tone: "danger" | "success") => {
      const markup = renderToStaticMarkup(
        <LoomarrProvider>
          <Notice body="Body" tone={tone} />
        </LoomarrProvider>,
      );
      const tag = /<[a-z]+[^>]*role="status"[^>]*>/.exec(markup)?.[0] ?? "";
      return declarationsOf(markup, tag);
    };

    expect(outline("success")).toContain("stateSuccess");
    expect(outline("danger")).toContain("stateDanger");
  });

  it("leads with the body when there is no title", () => {
    renderNotice(<Notice body="Secrets are redacted." tone="success" />);

    expect(screen.getByRole("status").textContent).toBe("Secrets are redacted.");
  });

  it("is an assertive alert only when asked", () => {
    renderNotice(<Notice body="The report failed." role="alert" tone="danger" />);

    const alert = screen.getByRole("alert");
    expect(alert.textContent).toBe("The report failed.");
    expect(alert.getAttribute("aria-live")).toBe("assertive");
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("runs its action from a press and from the keyboard", async () => {
    const onPress = vi.fn();
    const user = userEvent.setup();
    renderNotice(<Notice action={{ label: "Try again", onPress }} body="Body" title="Title" tone="warn" />);

    await user.tab();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Try again" }));
    await user.keyboard("{Enter}");
    expect(onPress).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole("button", { name: "Try again" }));
    expect(onPress).toHaveBeenCalledTimes(2);
  });
});
