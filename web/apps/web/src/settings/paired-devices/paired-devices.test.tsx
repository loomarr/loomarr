import {
  getDeviceListMockHandler,
  getDevicePairApproveMockHandler,
  getDeviceRevokeMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { PairedDevices } from "./paired-devices";

const makeWrapper = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

// ⚠ Every field is pinned. Generated handlers emit optional fields as `arrayElement([value,
// undefined])`, so a handler left on its defaults varies per CALL — `deviceName` and `lastSeenAt`
// are exactly what these assertions read, so unpinned they would make this suite flaky rather than
// merely arbitrary.
const shield = {
  id: "hash-living-room",
  deviceName: "Living Room Shield",
  createdAt: "2026-08-17T10:00:00Z",
  lastSeenAt: new Date().toISOString(),
};

const bedroom = {
  id: "hash-bedroom",
  deviceName: "Bedroom TV",
  createdAt: "2026-08-16T10:00:00Z",
  lastSeenAt: new Date().toISOString(),
};

describe("PairedDevices", () => {
  it("lists the devices signed in as this user", async () => {
    server.use(getDeviceListMockHandler({ devices: [shield, bedroom] }));
    render(<PairedDevices />, { wrapper: makeWrapper() });

    expect(await screen.findByText("Living Room Shield")).toBeInTheDocument();
    expect(screen.getByText("Bedroom TV")).toBeInTheDocument();
  });

  it("tells an operator with no devices how to add one", async () => {
    server.use(getDeviceListMockHandler({ devices: [] }));
    render(<PairedDevices />, { wrapper: makeWrapper() });

    expect(await screen.findByText("No devices yet")).toBeInTheDocument();
  });

  // The approval half: typing the code the TV displays confirms which device was approved, so the
  // operator can tell they approved the right one.
  it("confirms the device name after approving a code", async () => {
    server.use(
      getDeviceListMockHandler({ devices: [] }),
      getDevicePairApproveMockHandler({ deviceName: "Living Room Shield" }),
    );
    render(<PairedDevices />, { wrapper: makeWrapper() });

    await userEvent.type(screen.getByLabelText("Code shown on the device"), "BCDF-GHJK");
    await userEvent.click(screen.getByRole("button", { name: "Approve device" }));

    expect(await screen.findByText(/is approved/)).toBeInTheDocument();
    expect(screen.getByText("Living Room Shield")).toBeInTheDocument();
  });

  // A wrong code is the common case — a mistyped character, or a code that sat on screen too long.
  // It must say what to do next rather than failing silently.
  it("explains a rejected code instead of failing silently", async () => {
    // A raw msw handler rather than the generated one: the generated handlers model SUCCESS
    // shapes, so an error arm has to be stated directly.
    server.use(
      getDeviceListMockHandler({ devices: [] }),
      http.post("*/v1/auth/device/approve", () =>
        HttpResponse.json({ title: "Code not found" }, { status: 404 }),
      ),
    );
    render(<PairedDevices />, { wrapper: makeWrapper() });

    await userEvent.type(screen.getByLabelText("Code shown on the device"), "BCDF-GHJK");
    await userEvent.click(screen.getByRole("button", { name: "Approve device" }));

    expect(await screen.findByText(/wrong or has expired/)).toBeInTheDocument();
  });

  // Revoking is the security-relevant action: it must reach the API with the device's id.
  it("revokes the device the operator chose", async () => {
    const revoked: string[] = [];
    server.use(
      getDeviceListMockHandler({ devices: [shield] }),
      getDeviceRevokeMockHandler(({ request }) => {
        revoked.push(new URL(request.url).pathname);
      }),
    );
    render(<PairedDevices />, { wrapper: makeWrapper() });

    await screen.findByText("Living Room Shield");
    await userEvent.click(screen.getByRole("button", { name: "Remove" }));

    await waitFor(() => expect(revoked).toHaveLength(1));
    expect(revoked[0]).toContain("hash-living-room");
  });

  it("does not submit an empty code", async () => {
    server.use(getDeviceListMockHandler({ devices: [] }));
    render(<PairedDevices />, { wrapper: makeWrapper() });

    expect(await screen.findByRole("button", { name: "Approve device" })).toBeDisabled();
  });
});
