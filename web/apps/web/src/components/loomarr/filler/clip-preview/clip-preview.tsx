import { clipMediaURL } from "@loomarr/core/clip-thumb";
import { VideoPlayer } from "@/components/ui/video-player";
import type { ClipPreviewProps } from "./clip-preview.type";

// Exact clip identity and media addressing belong here; controls stay in the shared player.
// A new hash mounts a fresh player, so a different clip cannot inherit the old playhead.
const ClipPreview = ({ clip, onPlaybackStart, leading }: ClipPreviewProps) => (
  <VideoPlayer
    key={clip.hash}
    src={clipMediaURL(clip.hash)}
    title={clip.name}
    autoPlay
    onPlaybackStart={onPlaybackStart}
    leading={leading}
  />
);

export { ClipPreview };
