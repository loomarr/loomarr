import { channelsApi, unwrap } from "@loomarr/api";
import { deviceProfile } from "../use-hls-player/device-profile";
import type { ChannelPlaySource } from "./channel-play-url.type";

// qualityHint caps what the server offers; the player still adapts among those renditions.
const qualityHint = (): "auto" | "720" | "480" => {
  const conn = (navigator as Navigator & { connection?: { saveData?: boolean; effectiveType?: string } })
    .connection;
  if (conn?.saveData) return "480";
  if (conn?.effectiveType === "2g" || conn?.effectiveType === "slow-2g") return "480";
  if (conn?.effectiveType === "3g") return "720";

  const px = Math.max(window.screen.width, window.screen.height) * (window.devicePixelRatio || 1);
  if (px <= 640) return "480";
  if (px <= 1280) return "720";
  return "auto";
};

// mintChannelPlaySource is shared by the active player and adjacent warmer so a warmed neighbor's
// exact signed URL can be reused for the real tune. A fresh signature would produce different
// asset cache keys even though the publication bytes are identical.
const mintChannelPlaySource = async (
  channelId: string,
  signal: AbortSignal,
): Promise<ChannelPlaySource | undefined> => {
  const response = await channelsApi.channelPlayUrl(
    channelId,
    deviceProfile(),
    { quality: qualityHint() },
    { signal },
  );
  const body = unwrap(response);
  const url = body?.relativeUrl || body?.url;
  if (!url) return undefined;
  return { url, expiresAt: new Date(body.expiresAt).getTime() };
};

export { mintChannelPlaySource, qualityHint };
