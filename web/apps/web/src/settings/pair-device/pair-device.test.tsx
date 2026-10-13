import { getDevicePairApproveMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { PairDevice } from "./pair-device";

const makeWrapper = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const signedIn = () =>
  server.use(
    http.get("*/v1/auth/me", () =>
      HttpResponse.json({ id: "u-kid", name: "kid", role: "member", disabled: false }),
    ),
  );

const signedOut = () =>
  server.use(http.get("*/v1/auth/me", () => HttpResponse.json({ title: "Unauthorized" }, { status: 401 })));

describe("PairDevice", () => {
  it("prefills the code from the URL without approving it", async () => {
    signedIn();
    let approvals = 0;
    server.use(
      http.post("*/v1/auth/device/approve", () => {
        approvals += 1;
        return HttpResponse.json({ deviceName: "Shield" });
      }),
    );
    render(<PairDevice initialCode="BCDF-GHJK" />, { wrapper: makeWrapper() });

    const field = await screen.findByLabelText("Code shown on the device");
    expect(field).toHaveValue("BCDF-GHJK");
    // ⚠ The consent property: arriving with ?code= must NEVER approve on its own, or any link a
    // person opens could pair a device silently.
    expect(approvals).toBe(0);
  });

  // ADR 0043: a paired device acts with its approver's real role, so an admin's device is an admin.
  it("says the device acts with your access and role", async () => {
    signedIn();
    render(<PairDevice initialCode="BCDF-GHJK" />, { wrapper: makeWrapper() });

    expect(await screen.findByText(/The device acts with your access and role\./)).toBeInTheDocument();
    expect(screen.queryByText(/member access/)).not.toBeInTheDocument();
  });

  it("approves on an explicit click and names the device", async () => {
    signedIn();
    server.use(getDevicePairApproveMockHandler({ deviceName: "Living Room Shield" }));
    render(<PairDevice initialCode="BCDF-GHJK" />, { wrapper: makeWrapper() });

    await userEvent.click(await screen.findByRole("button", { name: "Add device" }));

    expect(await screen.findByText(/Living Room Shield is ready/)).toBeInTheDocument();
  });

  it("explains a rejected code rather than failing silently", async () => {
    signedIn();
    server.use(
      http.post("*/v1/auth/device/approve", () =>
        HttpResponse.json({ title: "Code not found" }, { status: 404 }),
      ),
    );
    render(<PairDevice initialCode="BCDF-GHJK" />, { wrapper: makeWrapper() });

    await userEvent.click(await screen.findByRole("button", { name: "Add device" }));

    // The identical message for a wrong code AND an expired one is the deliberate guess-guard
    // in handleDeviceApprove (internal/api/deviceroutes.go) — never split, or the message
    // itself becomes an oracle for grinding codes.
    expect(await screen.findByText(/wrong or has expired/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Try again" })).not.toBeInTheDocument();
  });

  // Maintainer decision (#1817, pairing states): the rate limiter gets its own message rather
  // than the "wrong or expired" copy, which would tell someone to recheck a code that was never
  // wrong.
  it("names a rate limit instead of blaming the code", async () => {
    signedIn();
    server.use(
      http.post("*/v1/auth/device/approve", () =>
        HttpResponse.json({ title: "Too many attempts" }, { status: 429 }),
      ),
    );
    render(<PairDevice initialCode="BCDF-GHJK" />, { wrapper: makeWrapper() });

    await userEvent.click(await screen.findByRole("button", { name: "Add device" }));

    expect(
      await screen.findByText("Too many pairing codes tried. Wait a moment and try again."),
    ).toBeInTheDocument();
    expect(screen.queryByText(/wrong or has expired/)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Try again" })).not.toBeInTheDocument();
  });

  // Maintainer decision (#1817, pairing states): a 5xx or a dropped request is worth retrying
  // with the same code — unlike a bad or expired one, nothing about the code was the problem.
  it("offers to retry a 5xx with the same code, rather than blaming it", async () => {
    signedIn();
    let approvals = 0;
    server.use(
      http.post("*/v1/auth/device/approve", () => {
        approvals += 1;
        if (approvals === 1) return HttpResponse.json({ title: "Internal error" }, { status: 500 });
        return HttpResponse.json({ deviceName: "Shield" });
      }),
    );
    render(<PairDevice initialCode="BCDF-GHJK" />, { wrapper: makeWrapper() });

    await userEvent.click(await screen.findByRole("button", { name: "Add device" }));

    expect(
      await screen.findByText("Couldn't reach Loomarr. Check your connection and try again."),
    ).toBeInTheDocument();
    expect(screen.queryByText(/wrong or has expired/)).not.toBeInTheDocument();

    await userEvent.click(await screen.findByRole("button", { name: "Try again" }));

    expect(await screen.findByText(/Shield is ready/)).toBeInTheDocument();
    expect(approvals).toBe(2);
  });

  it("offers to retry a network failure with the same code", async () => {
    signedIn();
    server.use(http.post("*/v1/auth/device/approve", () => HttpResponse.error()));
    render(<PairDevice initialCode="BCDF-GHJK" />, { wrapper: makeWrapper() });

    await userEvent.click(await screen.findByRole("button", { name: "Add device" }));

    expect(
      await screen.findByText("Couldn't reach Loomarr. Check your connection and try again."),
    ).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: "Try again" })).toBeInTheDocument();
  });

  // Signed-out is an expected arrival, not an error: the person holding the remote often is not
  // signed in on the phone they are typing on.
  it("offers sign-in and keeps the code in the return link", async () => {
    signedOut();
    render(<PairDevice initialCode="BCDF-GHJK" />, { wrapper: makeWrapper() });

    const link = await screen.findByRole("link", { name: "Sign in" });
    expect(link).toHaveAttribute("href", expect.stringContaining("%2Fpair%3Fcode%3DBCDF-GHJK"));
    expect(screen.getByText("BCDF-GHJK")).toBeInTheDocument();
  });

  it("does not submit an empty code", async () => {
    signedIn();
    render(<PairDevice />, { wrapper: makeWrapper() });

    expect(await screen.findByRole("button", { name: "Add device" })).toBeDisabled();
  });
});
