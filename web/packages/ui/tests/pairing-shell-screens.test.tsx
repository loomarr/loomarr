// @vitest-environment jsdom

import { PairingSession } from "@loomarr/core/pairing";
import type { ServerDiscovery } from "@loomarr/core/server-discovery";
import { LoomarrProvider } from "@loomarr/design-system";
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { PairingShell } from "../index";

// Records which Action asks for the TV's initial focus; react-native-web drops the prop from the DOM.
const preferredFocus = vi.hoisted(() => ({ labels: [] as string[] }));
vi.mock("@loomarr/design-system", async (importOriginal) => {
  const original = await importOriginal<typeof import("@loomarr/design-system")>();
  return {
    ...original,
    Action: (props: Parameters<typeof original.Action>[0]) => {
      if (props.hasTVPreferredFocus && typeof props.children === "string")
        preferredFocus.labels.push(props.children);
      return createElement(original.Action, props);
    },
  };
});

(
  globalThis as typeof globalThis & {
    IS_REACT_ACT_ENVIRONMENT: boolean;
  }
).IS_REACT_ACT_ENVIRONMENT = true;

const store = () => ({
  clear: vi.fn(async () => undefined),
  read: vi.fn(async () => undefined),
  write: vi.fn(async () => undefined),
});

const awaitingTv = async () => {
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
    store: store(),
  });
  const pairing = session.pair("https://loomarr.example.com");
  await vi.waitFor(() => expect(session.snapshot().status).toBe("awaiting-approval"));
  return { pairing, session };
};

describe("server address display", () => {
  const discoveredMarkup = async (
    density: "touch" | "tv",
    servers: { id: string; name: string; url: string }[],
  ) => {
    const session = new PairingSession({
      createTransport: vi.fn(),
      deviceName: "Living Room TV",
      store: store(),
    });
    await session.initialize(undefined);
    const discovery: ServerDiscovery = {
      snapshot: () => ({ servers, status: "searching" }),
      start: vi.fn(),
      stop: vi.fn(),
      subscribe: () => () => undefined,
    };
    return renderToStaticMarkup(
      <LoomarrProvider theme="dark">
        <PairingShell
          allowServerEntry
          density={density}
          discovery={discovery}
          discoveryForeground
          renderPaired={() => null}
          session={session}
        />
      </LoomarrProvider>,
    );
  };

  it("shows host[:port] without a scheme or trailing slash", async () => {
    const markup = await discoveredMarkup("touch", [
      { id: "a", name: "Loomarr on media-box", url: "https://loomarr.example.test" },
      { id: "b", name: "Loomarr on attic", url: "http://192.0.2.10:8080/" },
    ]);
    expect(markup).toContain("loomarr.example.test");
    expect(markup).toContain("192.0.2.10:8080");
    expect(markup).not.toContain("https://loomarr");
    expect(markup).not.toContain("http://192.0.2.10");
    expect(markup).not.toContain("192.0.2.10:8080/");
  });

  it("lists a discovered server the same way on the TV", async () => {
    const markup = await discoveredMarkup("tv", [
      { id: "a", name: "Loomarr on media-box", url: "https://media-box.example.test:8443" },
    ]);
    expect(markup).toContain("media-box.example.test:8443");
    expect(markup).not.toContain("https://media-box");
  });
});

describe("Apple TV code screen", () => {
  it("is titled and starts focus on Use a different server, never Get a new code", async () => {
    const { pairing, session } = await awaitingTv();
    preferredFocus.labels.length = 0;
    const markup = renderToStaticMarkup(
      <LoomarrProvider theme="dark">
        <PairingShell density="tv" renderPaired={() => null} session={session} />
      </LoomarrProvider>,
    );
    expect(markup).toContain("Approve this TV");
    expect(preferredFocus.labels).toContain("Use a different server");
    expect(preferredFocus.labels).not.toContain("Get a new code");
    session.stop();
    await pairing.catch(() => undefined);
  });
});

describe("a mistyped server address", () => {
  it("stays on server choice with the text kept and the message under the field", async () => {
    const session = new PairingSession({ createTransport: vi.fn(), deviceName: "iPhone", store: store() });
    const container = (
      globalThis as unknown as {
        document: { createElement: (tag: string) => Parameters<typeof createRoot>[0] };
      }
    ).document.createElement("div");
    const root = createRoot(container);
    act(() =>
      root.render(
        <LoomarrProvider theme="dark">
          <PairingShell allowServerEntry density="touch" renderPaired={() => null} session={session} />
        </LoomarrProvider>,
      ),
    );
    await vi.waitFor(() => expect(session.snapshot().status).toBe("needs-server"));
    await act(async () => {
      await session.pair("localhost:999999");
    });
    const page = container as unknown as {
      querySelector(selector: string): { value: string } | null;
      textContent: string | null;
    };
    expect(session.snapshot()).toMatchObject({
      input: "localhost:999999",
      kind: "invalid-address",
      retryable: false,
    });
    expect(page.textContent).toContain("Enter a Loomarr address like");
    expect(page.textContent).not.toContain("Couldn’t connect");
    expect(page.textContent).not.toContain("Try again");
    expect(page.querySelector("input")?.value).toBe("localhost:999999");
    act(() => root.unmount());
    session.stop();
  });
});
