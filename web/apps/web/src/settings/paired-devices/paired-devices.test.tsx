import {
  getDeviceListMockHandler,
  getDevicePairApproveMockHandler,
  getDeviceRevokeMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";
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

const signedInAs = (role: "admin" | "member") =>
  server.use(
    http.get("*/v1/auth/me", () => HttpResponse.json({ id: `u-${role}`, name: role, role, disabled: false })),
  );

// The page reads the role (an admin also gets Code Mode switches); a member unless a test says.
beforeEach(() => signedInAs("member"));

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

  // ADR 0043: a paired device acts with its approver's real role, so an admin's device is an admin.
  // The copy must not promise a member cap the server no longer applies.
  it("says the device acts with your access and role", async () => {
    server.use(getDeviceListMockHandler({ devices: [] }));
    render(<PairedDevices />, { wrapper: makeWrapper() });

    expect(await screen.findByText(/device acts with your access and role/)).toBeInTheDocument();
    expect(screen.queryByText(/not admin access/)).not.toBeInTheDocument();
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
    // The first press only asks: a removed device has to pair again.
    expect(screen.getByText(/Remove Living Room Shield\?/)).toBeInTheDocument();
    expect(revoked).toHaveLength(0);
    await userEvent.click(screen.getByRole("button", { name: "Remove device" }));

    await waitFor(() => expect(revoked).toHaveLength(1));
    expect(revoked[0]).toContain("hash-living-room");
  });

  it("does not submit an empty code", async () => {
    server.use(getDeviceListMockHandler({ devices: [] }));
    render(<PairedDevices />, { wrapper: makeWrapper() });

    expect(await screen.findByRole("button", { name: "Approve device" })).toBeDisabled();
  });

  it("blocks a blank code even when the form is submitted directly", async () => {
    let approvals = 0;
    server.use(
      getDeviceListMockHandler({ devices: [] }),
      http.post("*/v1/auth/device/approve", () => {
        approvals += 1;
        return HttpResponse.json({ deviceName: "Shield" });
      }),
    );
    render(<PairedDevices />, { wrapper: makeWrapper() });

    const field = await screen.findByLabelText("Code shown on the device");
    await userEvent.type(field, "   ");
    const form = field.closest("form");
    if (!form) throw new Error("the code field is not in a form");
    fireEvent.submit(form);

    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(approvals).toBe(0);
  });

  it("keeps a rejected code for correction and clears an approved one", async () => {
    let approvals = 0;
    server.use(
      getDeviceListMockHandler({ devices: [] }),
      http.post("*/v1/auth/device/approve", () => {
        approvals += 1;
        if (approvals === 1) return HttpResponse.json({ title: "Code not found" }, { status: 404 });
        return HttpResponse.json({ deviceName: "Shield" });
      }),
    );
    render(<PairedDevices />, { wrapper: makeWrapper() });

    const field = await screen.findByLabelText("Code shown on the device");
    await userEvent.type(field, "BCDF-GHJK");
    await userEvent.click(screen.getByRole("button", { name: "Approve device" }));
    expect(await screen.findByText(/wrong or has expired/)).toBeInTheDocument();
    expect(field).toHaveValue("BCDF-GHJK");

    await userEvent.click(screen.getByRole("button", { name: "Approve device" }));
    expect(await screen.findByText(/is approved/)).toBeInTheDocument();
    expect(field).toHaveValue("");
  });
});

// Code Mode (#4486) is an admin's per-device switch; a member never sees it, nor asks the server.
describe("PairedDevices: Code Mode", () => {
  it("shows an admin each device's switch as the server has it, and turns one on", async () => {
    signedInAs("admin");
    const sent: { id: string; on: unknown }[] = [];
    server.use(
      getDeviceListMockHandler({ devices: [shield, bedroom] }),
      http.get("*/v1/agents/code-mode", () => HttpResponse.json({ devices: ["hash-bedroom"] })),
      http.put("*/v1/agents/code-mode/:id", async ({ params, request }) => {
        sent.push({ id: String(params.id), on: ((await request.json()) as { on: unknown }).on });
        return new HttpResponse(null, { status: 204 });
      }),
    );
    render(<PairedDevices />, { wrapper: makeWrapper() });

    const switches = await screen.findAllByRole("switch", { name: "Code Mode" });
    expect(switches.map((s) => s.getAttribute("aria-checked"))).toEqual(["false", "true"]);
    await userEvent.click(switches[0] as HTMLElement);
    await waitFor(() => expect(sent).toEqual([{ id: "hash-living-room", on: true }]));
  });

  it("never shows a member the switch or asks for it", async () => {
    signedInAs("member");
    let asked = 0;
    server.use(
      getDeviceListMockHandler({ devices: [shield] }),
      http.get("*/v1/agents/code-mode", () => {
        asked += 1;
        return HttpResponse.json({ devices: [] });
      }),
    );
    render(<PairedDevices />, { wrapper: makeWrapper() });

    expect(await screen.findByText("Living Room Shield")).toBeInTheDocument();
    expect(screen.queryByRole("switch", { name: "Code Mode" })).not.toBeInTheDocument();
    expect(asked).toBe(0);
  });
});
