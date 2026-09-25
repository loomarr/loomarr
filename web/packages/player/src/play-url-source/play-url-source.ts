import { getChannelPlayUrlUrl, getListChannelsUrl } from "@loomarr/api/endpoints/channels";
import type { ListChannelsOutputBody } from "@loomarr/api/models/listChannelsOutputBody";
import type { PlayURLOutputBody } from "@loomarr/api/models/playURLOutputBody";
import type { PlayerChannel, PlayerSourcePort } from "../player-source";
import { warmSource } from "../warm-source";

interface PlayUrlSourceOptions {
  /** Normalized URL of the paired Loomarr server. */
  baseUrl: string;
  /** Authenticated request function owned by the active client session. */
  fetch: typeof globalThis.fetch;
}

interface ChannelCatalogPort {
  list: (signal: AbortSignal) => Promise<readonly PlayerChannel[]>;
}

const trimLeadingSlashes = (value: string) => {
  let start = 0;
  while (start < value.length && value.charCodeAt(start) === 47) start += 1;
  return value.slice(start);
};

const trimTrailingSlashes = (value: string) => {
  let end = value.length;
  while (end > 0 && value.charCodeAt(end - 1) === 47) end -= 1;
  return value.slice(0, end);
};

const resolveStreamUrl = (
  baseUrl: string,
  response: Pick<PlayURLOutputBody, "relativeUrl" | "url">,
): string => {
  if (response.relativeUrl.trim()) {
    return `${trimTrailingSlashes(baseUrl)}/${trimLeadingSlashes(response.relativeUrl)}`;
  }
  if (response.url.trim()) return response.url;
  throw new Error("This Loomarr returned no stream address for the channel.");
};

/** The still address, resolved like the stream's; undefined when the server sent none (an older server). */
const resolveStillUrl = (
  baseUrl: string,
  response: Partial<Pick<PlayURLOutputBody, "relativeStillUrl" | "stillUrl">>,
): string | undefined => {
  if (response.relativeStillUrl?.trim()) {
    return `${trimTrailingSlashes(baseUrl)}/${trimLeadingSlashes(response.relativeStillUrl)}`;
  }
  return response.stillUrl?.trim() || undefined;
};

const createPlayUrlSourcePort = ({ baseUrl, fetch: request }: PlayUrlSourceOptions): PlayerSourcePort => {
  const mint: PlayerSourcePort["mint"] = async (channel, profile, signal) => {
    const response = await request(getChannelPlayUrlUrl(channel.id), {
      body: JSON.stringify(profile),
      headers: { "Content-Type": "application/json" },
      method: "POST",
      signal,
    });
    if (!response.ok) throw new Error(`Couldn't mint a play URL (${response.status}).`);

    const body = (await response.json()) as PlayURLOutputBody;
    const expiry = Date.parse(body.expiresAt);
    const headerServerTime = Date.parse(response.headers.get("Date") ?? "");
    const serverTime = Number.isFinite(body.serverTimeMs) ? body.serverTimeMs : headerServerTime;
    const stillUri = resolveStillUrl(baseUrl, body);
    return {
      expiresAt: Number.isFinite(expiry) ? expiry : undefined,
      ...(Number.isFinite(serverTime) ? { serverTimeMs: serverTime } : {}),
      ...(stillUri ? { stillUri } : {}),
      uri: resolveStreamUrl(baseUrl, body),
    };
  };

  return {
    mint,
    // Warms on the exact signed URL the real tune will reuse (see warmSource for the protocol).
    warm: async (channel, profile, signal) => {
      const minted = await mint(channel, profile, signal);
      const warmed = await warmSource(minted.uri, (url) => request(url, { method: "GET", signal }));
      return { ...minted, warmed };
    },
  };
};

const createChannelCatalogPort = (request: typeof globalThis.fetch): ChannelCatalogPort => ({
  list: async (signal) => {
    const response = await request(getListChannelsUrl(), { method: "GET", signal });
    if (!response.ok) throw new Error(`Couldn't load channels (${response.status}).`);
    const body = (await response.json()) as ListChannelsOutputBody;
    return body.channels.map(({ id, inAppPlayable, name, number }) => ({
      id,
      inAppPlayable,
      name,
      number,
    }));
  },
});

export type { ChannelCatalogPort, PlayUrlSourceOptions };
export { createChannelCatalogPort, createPlayUrlSourcePort, resolveStreamUrl };
