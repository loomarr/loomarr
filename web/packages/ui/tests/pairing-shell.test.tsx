import { PairingSession } from "@loomarr/core/pairing";
import type { ServerDiscovery } from "@loomarr/core/server-discovery";
import { LoomarrProvider } from "@loomarr/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { PairingShell } from "../index";

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
  const pairing = session.pair("https://loomarr.projectguacamole.com");
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
          renderPaired={() => null}
          session={session}
        />
      </LoomarrProvider>,
    );

    expect(markup).toContain("Find your Loomarr server");
    expect(markup).toContain("Connect to Loomarr on media-box");
    expect(markup).toContain("Enter address manually");
    expect(markup).not.toContain("EXPO_PUBLIC_LOOMARR_URL");
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
});
