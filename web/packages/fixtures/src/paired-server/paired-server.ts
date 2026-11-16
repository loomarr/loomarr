import { guideChannels, guideFrom, guideTo } from "../testcard";

// A paired Loomarr server for a native client's test: the REST answers the paired runtime asks for
// (catalog, guide, my channels, version, play URL) and the live event stream it holds open, built from
// the test card's guide. The client runs as shipped; only the network is this.

const pairedServerUrl = "https://loomarr.test";

/** One connection the client opened to the server's event stream (the native stream is an XMLHttpRequest). */
class PairedServerEventRequest {
  aborted = false;
  onerror: (() => void) | null = null;
  onreadystatechange: (() => void) | null = null;
  readyState = 1;
  responseText = "";
  status = 0;

  constructor(readonly connections: PairedServerEventRequest[]) {}

  abort() {
    this.aborted = true;
  }

  open() {
    this.connections.push(this);
  }

  send() {
    this.status = 200;
    this.readyState = 3;
    this.onreadystatechange?.();
  }

  setRequestHeader() {}

  /** The server sends one frame down this connection. */
  frame(type: string, data: unknown) {
    this.responseText += `event: ${type}\ndata: ${JSON.stringify(data)}\n\n`;
    this.onreadystatechange?.();
  }
}

const json = (body: unknown) =>
  new Response(JSON.stringify(body), { headers: { "Content-Type": "application/json" }, status: 200 });

/**
 * The paired server. Stub `fetch` with `fetch` and `XMLHttpRequest` with `EventRequest`; `requests`
 * lists each call as "METHOD /path", and `sendFrame` sends an event frame to the open stream.
 */
const createPairedServer = () => {
  const connections: PairedServerEventRequest[] = [];
  const requests: string[] = [];
  let guideHeld: Promise<void> | undefined;
  let guideFails = false;
  const catalog = [...guideChannels]
    .sort((first, second) => first.number - second.number)
    .map(({ channelId, name, number }) => ({ id: channelId, inAppPlayable: true, name, number }));

  const fetch = async (input: string, init?: RequestInit): Promise<Response> => {
    const { pathname } = new URL(input, pairedServerUrl);
    requests.push(`${init?.method ?? "GET"} ${pathname}`);
    if (pathname === "/v1/channels") return json({ channels: catalog });
    if (pathname === "/v1/guide") {
      await guideHeld;
      if (guideFails) return new Response(null, { status: 503 });
      return json({ channels: guideChannels, fromMs: guideFrom, timezone: "UTC", toMs: guideTo });
    }
    if (pathname === "/v1/me/channels") return json({ favourites: [], recent: [] });
    if (pathname === "/v1/system/version") return json({ dirty: false, version: "1.2.3" });
    if (pathname.endsWith("/play-url")) {
      return json({
        expiresAt: "2030-01-01T00:00:00Z",
        relativeUrl: "/v1/playout/hls/channel/master.m3u8",
        serverTimeMs: Date.now(),
        url: "",
      });
    }
    if (pathname.endsWith(".m3u8")) return new Response("#EXTM3U\n", { status: 200 });
    return json({});
  };

  return {
    EventRequest: class extends PairedServerEventRequest {
      constructor() {
        super(connections);
      }
    },
    /** The channels the server lists, lowest number first. */
    catalog,
    /** From now until the returned restore is called, the guide answers 503 (after any hold). */
    failGuide: () => {
      guideFails = true;
      return () => {
        guideFails = false;
      };
    },
    fetch,
    /** Counts the guide loads so far. */
    guideLoads: () => requests.filter((request) => request === "GET /v1/guide").length,
    /** From now until the returned release is called, the guide's answer is on its way. */
    holdGuide: () => {
      let release: () => void = () => undefined;
      guideHeld = new Promise<void>((resolve) => {
        release = resolve;
      });
      return release;
    },
    requests,
    sendFrame: (type: string, data: unknown) => connections.at(-1)?.frame(type, data),
    serverUrl: pairedServerUrl,
  };
};

export { createPairedServer, pairedServerUrl };
