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

    expect(await screen.findByText(/wrong or has expired/)).toBeInTheDocument();
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
