import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { RestartOverlay } from "./restart-overlay";

describe("RestartOverlay", () => {
  // ⚠ THE POINT: while the server is down every request fails, so the app must not accept
  // clicks that were never going to work. `cursor-progress` marks it busy; the fixed
  // full-screen layer is what actually swallows the input.
  it("covers the app and blocks interaction while restarting", () => {
    render(<RestartOverlay restarting />);

    const overlay = screen.getByRole("alertdialog");
    expect(overlay).toHaveClass("fixed", "inset-0");
    expect(overlay).toHaveClass("cursor-progress");
    expect(overlay).not.toHaveClass("pointer-events-none");
    expect(overlay).toHaveTextContent(/restarting loomarr/i);
  });

  // ⚠ Once the app is usable the overlay must stop swallowing clicks, or the brief
  // confirmation would eat the operator's next action.
  it("stops blocking once the app is back", () => {
    render(<RestartOverlay restarting={false} justCameBack />);

    const overlay = screen.getByRole("alertdialog");
    expect(overlay).toHaveClass("pointer-events-none");
    expect(overlay).toHaveTextContent(/loomarr is back/i);
  });

  // A restart that never returned needs the operator to do something, so it keeps the
  // screen rather than fading like a success.
  it("keeps blocking when the app never came back", () => {
    render(<RestartOverlay restarting={false} failed="Loomarr hasn't come back." />);

    const overlay = screen.getByRole("alertdialog");
    expect(overlay).toHaveClass("cursor-progress");
    expect(overlay).toHaveTextContent(/hasn't come back/i);
  });

  it("renders nothing when idle", () => {
    const { container } = render(<RestartOverlay restarting={false} />);
    expect(container).toBeEmptyDOMElement();
  });
});
