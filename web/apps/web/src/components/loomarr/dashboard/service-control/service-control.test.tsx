import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ServiceControl } from "./service-control";

const noop = () => {};
const idle = { available: true, streamingChannels: 0, restartRequired: false };

describe("ServiceControl", () => {
  // ⚠ Restart is two-step. The consequences are the whole reason the dialog exists, so a
  // single click must not reach the server.
  it("does not restart until the operator confirms", async () => {
    const onRestart = vi.fn();
    render(<ServiceControl cost={idle} onRestart={onRestart} />);

    await userEvent.click(screen.getByRole("button", { name: /^Restart\.\.\./ }));
    expect(onRestart).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole("button", { name: /^Restart now$/ }));
    expect(onRestart).toHaveBeenCalledOnce();
  });

  it("can be cancelled without restarting", async () => {
    const onRestart = vi.fn();
    render(<ServiceControl cost={idle} onRestart={onRestart} />);

    await userEvent.click(screen.getByRole("button", { name: /^Restart\.\.\./ }));
    await userEvent.click(screen.getByRole("button", { name: /cancel/i }));

    expect(onRestart).not.toHaveBeenCalled();
    expect(screen.queryByText(/here's exactly what happens/i)).not.toBeInTheDocument();
  });

  // ⚠ THE COPY GATE. The mock promises "Channels keep playing — Tunarr streams them, not
  // Loomarr", which §9.1 records as FALSE once Loomarr owns the encoder. The dialog must
  // say how many channels actually drop, from the live count.
  it("says how many channels a restart will drop", async () => {
    render(
      <ServiceControl
        cost={{ available: true, streamingChannels: 3, restartRequired: false }}
        onRestart={noop}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: /^Restart\.\.\./ }));

    expect(
      screen.getByText(/Restart now\? 3 channels Loomarr is streaming will cut out/i),
    ).toBeInTheDocument();
  });

  // A Tunarr-backed install streams nothing itself, so there is no drop to warn about —
  // and inventing one would be the mirror-image lie.
  it("omits the drop warning when nothing is streaming", async () => {
    render(<ServiceControl cost={idle} onRestart={noop} />);

    await userEvent.click(screen.getByRole("button", { name: /^Restart\.\.\./ }));

    expect(screen.queryByText(/will cut out/i)).not.toBeInTheDocument();
    expect(screen.getByText("Restart now?")).toBeInTheDocument();
  });

  // ⚠ Explained, not hidden. Hiding the control would leave an operator hunting for a
  // feature the docs describe.
  it("explains when this build cannot restart itself", () => {
    render(
      <ServiceControl
        cost={{ available: false, streamingChannels: 0, restartRequired: false }}
        onRestart={noop}
      />,
    );

    expect(screen.getByText(/can't restart itself/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Restart\.\.\./ })).not.toBeInTheDocument();
  });

  it("surfaces a failure where it happened", () => {
    render(<ServiceControl cost={idle} onRestart={noop} error="Couldn't restart." />);
    expect(screen.getByRole("alert")).toHaveTextContent("Couldn't restart.");
  });
});
