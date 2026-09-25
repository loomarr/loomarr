import { warmSource } from "@loomarr/player";
import { mintChannelPlaySource } from "../channel-play-url";
import type { WarmedChannel } from "./channel-warmer.type";

// warmChannel never creates a player, MediaSource, or decoder. The warm protocol (prepared probe,
// bounded live warm on a clean miss, init + newest fragment) is the shared player package's
// `warmSource`, so the browser and the TV warm identically; this adds only the browser's transport
// (same-origin credentials, HTTP cache for the immutable assets) and keeps the exact normal signed
// URL for the real tune. Capacity and errors remain harmless speculative misses.
const warmChannel = async (channelId: string, signal: AbortSignal): Promise<WarmedChannel | undefined> => {
  const source = await mintChannelPlaySource(channelId, signal);
  if (!source) return undefined;
  const warmed = await warmSource(new URL(source.url, window.location.href).toString(), (url, kind) =>
    fetch(url, {
      signal,
      credentials: "same-origin",
      cache: kind === "playlist" ? "no-store" : "force-cache",
    }),
  );
  return { ...source, warmed };
};

export { warmChannel };
