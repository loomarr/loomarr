// @vitest-environment jsdom

import { PairingSession } from "@loomarr/core/pairing";
import type { ServerDiscovery } from "@loomarr/core/server-discovery";
import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { PairingShell } from "../index";

(
  globalThis as typeof globalThis & {
    IS_REACT_ACT_ENVIRONMENT: boolean;
  }
).IS_REACT_ACT_ENVIRONMENT = true;

const createTestContainer = () =>
  (
    globalThis as unknown as {
      document: { createElement: (tagName: string) => Parameters<typeof createRoot>[0] };
    }
  ).document.createElement("div");

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

const awaitingSessionWithSeconds = async (seconds: number) => {
  // Round to the second first: `serverDate` only carries second precision (`toUTCString`), so an
  // unrounded `now` would leave `pairingLifetimeSeconds` off by a fractional second.
  const now = Math.floor(Date.now() / 1_000) * 1_000;
  const session = new PairingSession({
    createTransport: () => ({
      poll: vi.fn(async () => ({ status: "pending" as const })),
      start: vi.fn(async () => ({
        body: {
          deviceCode: "device-code",
          expiresAt: new Date(now + seconds * 1_000).toISOString(),
          interval: 5,
          userCode: "WMQJ-QVFJ",
        },
        serverDate: new Date(now).toUTCString(),
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

describe("TV pairing offer", () => {
  it("offers discovered servers before the manual-address fallback", async () => {
    const session = new PairingSession({
      createTransport: vi.fn(),
      deviceName: "Living Room TV",
      store: {
        clear: vi.fn(async () => undefined),
        read: vi.fn(async () => undefined),
        write: vi.fn(async () => undefined),
      },
    });
    await session.initialize(undefined);
    const discovery: ServerDiscovery = {
      snapshot: () => ({
        servers: [{ id: "living-room", name: "Loomarr on media-box", url: "http://192.0.2.10:8080" }],
        status: "searching",
      }),
      start: vi.fn(),
      stop: vi.fn(),
      subscribe: () => () => undefined,
    };

    const markup = renderToStaticMarkup(
      <LoomarrProvider theme="dark">
        <PairingShell
          allowServerEntry
          density="tv"
          discovery={discovery}
          discoveryForeground
          renderPaired={() => null}
          session={session}
        />
      </LoomarrProvider>,
    );

    expect(markup).toContain("Find your Loomarr server");
    expect(markup).toContain("Connect to Loomarr on media-box");
    expect(markup).toContain("http://192.0.2.10:8080");
    expect(markup).toContain("Enter address manually");
    expect(markup).not.toContain("EXPO_PUBLIC_LOOMARR_URL");
  });

  it("names the device by its density on the server-choice screen", async () => {
    const session = new PairingSession({
      createTransport: vi.fn(),
      deviceName: "iPhone",
      store: {
        clear: vi.fn(async () => undefined),
        read: vi.fn(async () => undefined),
        write: vi.fn(async () => undefined),
      },
    });
    await session.initialize(undefined);
    const render = (density: "tv" | "touch") =>
      renderToStaticMarkup(
        <LoomarrProvider theme="dark">
          <PairingShell allowServerEntry density={density} renderPaired={() => null} session={session} />
        </LoomarrProvider>,
      );

    expect(render("tv")).toContain("You’ll approve this TV on the next screen.");
    const touch = render("touch");
    expect(touch).toContain("You’ll approve this device on the next screen.");
    expect(touch).not.toContain("TV");
  });

  it("browses only while the unpaired connection screen is foregrounded", async () => {
    const session = new PairingSession({
      createTransport: vi.fn(),
      deviceName: "Living Room TV",
      store: {
        clear: vi.fn(async () => undefined),
        read: vi.fn(async () => undefined),
        write: vi.fn(async () => undefined),
      },
    });
    await session.initialize(undefined);
    const discoverySnapshot = { servers: [], status: "searching" as const };
    const discovery: ServerDiscovery = {
      snapshot: () => discoverySnapshot,
      start: vi.fn(),
      stop: vi.fn(),
      subscribe: () => () => undefined,
    };
    const container = createTestContainer();
    const root = createRoot(container);
    const render = (discoveryForeground: boolean) =>
      root.render(
        <LoomarrProvider theme="dark">
          <PairingShell
            allowServerEntry
            density="tv"
            discovery={discovery}
            discoveryForeground={discoveryForeground}
            renderPaired={() => null}
            session={session}
          />
        </LoomarrProvider>,
      );

    act(() => render(true));
    expect(discovery.start).toHaveBeenCalledOnce();
    expect(discovery.stop).not.toHaveBeenCalled();

    act(() => render(false));
    expect(discovery.stop).toHaveBeenCalled();

    act(() => root.unmount());
  });

  it("keeps manual address entry available when automatic discovery times out", async () => {
    const session = new PairingSession({
      createTransport: vi.fn(),
      deviceName: "Living Room TV",
      store: {
        clear: vi.fn(async () => undefined),
        read: vi.fn(async () => undefined),
        write: vi.fn(async () => undefined),
      },
    });
    await session.initialize(undefined);
    const discovery: ServerDiscovery = {
      snapshot: () => ({
        error: "Couldn't find a Loomarr server. You can still enter the address manually.",
        servers: [],
        status: "unavailable",
      }),
      start: vi.fn(),
      stop: vi.fn(),
      subscribe: () => () => undefined,
    };

    const markup = renderToStaticMarkup(
      <LoomarrProvider theme="dark">
        <PairingShell
          allowServerEntry
          density="tv"
          discovery={discovery}
          discoveryForeground
          renderPaired={() => null}
          session={session}
        />
      </LoomarrProvider>,
    );

    expect(markup).toContain("Couldn&#x27;t find a Loomarr server");
    expect(markup).toContain("Enter address manually");
  });

  it("keeps the Loomarr mark in the living-room QR code", async () => {
    const { pairing, session } = await awaitingSession();

    const markup = renderToStaticMarkup(
      <LoomarrProvider theme="dark">
        <PairingShell density="tv" renderPaired={() => null} session={session} />
      </LoomarrProvider>,
    );

    // The screen-level brand mark, QR matrix, and protected QR centre mark are separate SVGs.
    expect(markup.match(/<svg/g)).toHaveLength(3);

    session.stop();
    await pairing;
  });

  // Maintainer decision (#1817, pairing states): the countdown turns urgent — danger colour,
  // semibold — under 60 seconds, not at or above it.
  it("turns the countdown urgent under 60 seconds", async () => {
    const { pairing, session } = await awaitingSessionWithSeconds(59);

    const markup = renderToStaticMarkup(
      <LoomarrProvider theme="dark">
        <PairingShell density="tv" renderPaired={() => null} session={session} />
      </LoomarrProvider>,
    );

    expect(markup).toContain("Expires in 0:59");
    expect(markup).toMatch(/col-stateDanger/);

    session.stop();
    await pairing;
  });

  it("keeps the countdown plain at 60 seconds", async () => {
    const { pairing, session } = await awaitingSessionWithSeconds(60);

    const markup = renderToStaticMarkup(
      <LoomarrProvider theme="dark">
        <PairingShell density="tv" renderPaired={() => null} session={session} />
      </LoomarrProvider>,
    );

    expect(markup).toContain("Expires in 1:00");
    expect(markup).not.toMatch(/col-stateDanger/);

    session.stop();
    await pairing;
  });

  it("does not show a refresh notice next to a device's first code", async () => {
    const { pairing, session } = await awaitingSession();

    const markup = renderToStaticMarkup(
      <LoomarrProvider theme="dark">
        <PairingShell density="tv" renderPaired={() => null} session={session} />
      </LoomarrProvider>,
    );

    expect(markup).not.toContain("New code");

    session.stop();
    await pairing;
  });

  it("names the new code when the session silently replaced an expired one", async () => {
    let starts = 0;
    const session = new PairingSession({
      createTransport: () => ({
        // The first poll finds the code dead; every poll after that is left permanently
        // in-flight (like a real long poll with nothing to report yet) instead of resolving
        // to "pending" repeatedly — a pending result that keeps resolving would re-loop with
        // no macrotask in between, starving this test's own timers.
        poll: vi.fn(() =>
          starts === 1
            ? Promise.resolve({ status: "expired" as const })
            : new Promise<never>(() => undefined),
        ),
        start: vi.fn(async () => {
          starts += 1;
          return {
            body: {
              deviceCode: `device-code-${starts}`,
              expiresAt: new Date(Date.now() + 600_000).toISOString(),
              interval: 5,
              userCode: `CODE-${starts}`,
            },
            serverDate: new Date().toUTCString(),
          };
        }),
      }),
      deviceName: "Living Room TV",
      sleep: async () => undefined,
      store: {
        clear: vi.fn(async () => undefined),
        read: vi.fn(async () => undefined),
        write: vi.fn(async () => undefined),
      },
    });
    const pairing = session.pair("https://loomarr.example.com");
    await vi.waitFor(() =>
      expect(session.snapshot()).toMatchObject({ status: "awaiting-approval", userCode: "CODE-2" }),
    );

    const markup = renderToStaticMarkup(
      <LoomarrProvider theme="dark">
        <PairingShell density="tv" renderPaired={() => null} session={session} />
      </LoomarrProvider>,
    );

    expect(markup).toContain("New code — the last one expired.");

    // The mock poll above is deliberately a promise that never settles (simulating a long
    // poll in flight), so `stop()` leaves `pairing` permanently pending rather than rejecting
    // it — nothing here awaits it.
    session.stop();
    void pairing;
  });
});
