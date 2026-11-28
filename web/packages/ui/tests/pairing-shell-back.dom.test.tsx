// @vitest-environment jsdom

import { PairingSession } from "@loomarr/core/pairing";
import { LoomarrProvider } from "@loomarr/design-system";
import { act, cleanup, renderUi } from "@loomarr/test-support";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const backHandlers = vi.hoisted(() => new Set<() => boolean | null | undefined>());

vi.mock("react-native", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-native")>();
  return {
    ...actual,
    BackHandler: {
      ...actual.BackHandler,
      addEventListener: (_event: string, handler: () => boolean | null | undefined) => {
        backHandlers.add(handler);
        return { remove: () => backHandlers.delete(handler) };
      },
    },
  };
});

const { PairingShell, pairingBackAvailable } = await import("../pairing");

// A root the test renders into more than once, unmounted by `cleanup`.
const testRoot = () => {
  const { rerender, unmount } = renderUi(null);
  return { render: rerender, unmount };
};
afterEach(cleanup);

const awaitingSession = async () => {
  const session = new PairingSession({
    createTransport: () => ({
      poll: vi.fn(async () => ({ status: "pending" as const })),
      start: vi.fn(async () => ({
        body: {
          deviceCode: "device-code",
          expiresAt: new Date(Date.now() + 600_000).toISOString(),
          interval: 5,
          userCode: "WMQJ-QVFJ",
        },
        serverDate: new Date().toUTCString(),
      })),
    }),
    deviceName: "Living Room TV",
    sleep: (_milliseconds, signal) =>
      new Promise((_resolve, reject) => {
        signal.addEventListener("abort", () => reject(new Error("Pairing stopped")));
      }),
    store: {
      clear: vi.fn(async () => undefined),
      read: vi.fn(async () => undefined),
      write: vi.fn(async () => undefined),
    },
  });
  const pairing = session.pair("https://loomarr.example.com");
  await vi.waitFor(() => expect(session.snapshot().status).toBe("awaiting-approval"));
  return { pairing, session };
};

describe("pairing Back (#1500 Apple TV audit #1, #13)", () => {
  beforeEach(() => {
    backHandlers.clear();
  });

  it("has a way back only from the code screen and a failed connection", () => {
    expect(pairingBackAvailable({ status: "loading" })).toBe(false);
    expect(pairingBackAvailable({ status: "needs-server" })).toBe(false);
    expect(
      pairingBackAvailable({
        deviceCode: "device-code",
        expiresAtMs: 0,
        refreshedFromExpired: false,
        serverUrl: "https://loomarr.example.com",
        status: "awaiting-approval",
        userCode: "WMQJ-QVFJ",
        verificationUri: "https://loomarr.example.com/pair",
        verificationUriComplete: "https://loomarr.example.com/pair?code=WMQJ-QVFJ",
      }),
    ).toBe(true);
    expect(pairingBackAvailable({ message: "Unreachable", retryable: true, status: "failed" })).toBe(true);
    expect(pairingBackAvailable({ serverUrl: "https://loomarr.example.com", status: "revoked" })).toBe(false);
  });

  it("offers a TV on the code screen another server, as the phone does", async () => {
    const { pairing, session } = await awaitingSession();

    const markup = renderToStaticMarkup(
      <LoomarrProvider theme="dark">
        <PairingShell density="tv" renderPaired={() => null} session={session} />
      </LoomarrProvider>,
    );

    expect(markup).toContain("Get a new code");
    expect(markup).toContain("Use a different server");
    session.stop();
    await pairing;
  });

  it("sends Back from the code screen to server choice, then leaves the root to the platform", async () => {
    const { pairing, session } = await awaitingSession();
    const root = testRoot();

    act(() =>
      root.render(
        <LoomarrProvider theme="dark">
          <PairingShell allowServerEntry density="tv" renderPaired={() => null} session={session} />
        </LoomarrProvider>,
      ),
    );
    expect(backHandlers.size).toBe(1);

    let handled: boolean | null | undefined;
    act(() => {
      for (const handler of backHandlers) handled = handler();
    });

    expect(handled).toBe(true);
    expect(session.snapshot().status).toBe("needs-server");
    // Server choice is a root: no handler, so Back (and Apple TV's Menu) leaves the app.
    expect(backHandlers.size).toBe(0);
    act(() => root.unmount());
    await pairing;
  });
});
