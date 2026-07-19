import type { SessionBody } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SessionList } from "./session-list";

// A fixed clock, so "3d ago" is an assertion rather than a race with the wall clock.
const NOW = Date.parse("2026-07-19T12:00:00Z");
const hoursAgo = (h: number) => NOW - h * 3_600_000;
const hoursAhead = (h: number) => NOW + h * 3_600_000;

const session = (over: Partial<SessionBody> = {}): SessionBody => ({
  id: "hash-a",
  userId: "u1",
  createdAt: hoursAgo(2),
  expiresAt: hoursAhead(48),
  current: false,
  ...over,
});

afterEach(() => vi.useRealTimers());

const freeze = () => {
  vi.useFakeTimers();
  vi.setSystemTime(NOW);
};

describe("SessionList", () => {
  it("says plainly that nobody is signed in", () => {
    render(<SessionList userName="Ada" sessions={[]} />);
    expect(screen.getByText(/ada has no active sessions/i)).toBeInTheDocument();
  });

  it("agrees with the count at one", () => {
    freeze();
    render(<SessionList userName="Ada" sessions={[session()]} />);
    expect(screen.getByText(/1 active session for Ada/)).toBeInTheDocument();
  });

  it("shows when each session started and when it lapses", () => {
    freeze();
    render(<SessionList userName="Ada" sessions={[session()]} />);
    expect(screen.getByText(/signed in 2h ago/i)).toBeInTheDocument();
    expect(screen.getByText(/expires in 2d/i)).toBeInTheDocument();
  });

  // Labelled, never hidden: an admin reviewing their own account must see every live
  // session, and the label is what stops them signing themselves out by accident.
  it("marks the caller's own session and names the consequence", () => {
    freeze();
    render(<SessionList userName="Ada" sessions={[session({ current: true })]} />);
    expect(screen.getByText("This device")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /sign out \(this device\)/i })).toBeInTheDocument();
  });

  // Deliberately NOT on the frozen clock: userEvent schedules its own timers, and
  // vi.useFakeTimers() without advanceTimers deadlocks the click. This test asserts on
  // the handle, not on formatted time, so real timers are the simpler correct choice.
  it("revokes by the opaque handle the API returned", async () => {
    const onRevoke = vi.fn();
    render(<SessionList userName="Ada" sessions={[session({ id: "hash-xyz" })]} onRevoke={onRevoke} />);
    await userEvent.click(screen.getByRole("button", { name: "Sign out" }));
    expect(onRevoke).toHaveBeenCalledWith("hash-xyz");
  });

  it("spins only the row being revoked", () => {
    freeze();
    render(
      <SessionList userName="Ada" sessions={[session({ id: "a" }), session({ id: "b" })]} revoking="a" />,
    );
    const buttons = screen.getAllByRole("button", { name: /sign out/i });
    expect(buttons[0]).toBeDisabled();
    expect(buttons[1]).not.toBeDisabled();
  });
});
