interface IngestPanelProps {
  // Whether the running IMAGE carries the yt-dlp + ffmpeg tooling (§10, §16). This is the
  // one feature gate no setting can open — only running loomarr:filler can — so the copy
  // must point at the image, never at Settings.
  ingestAvailable: boolean;
  onIngested?: () => void;
  className?: string;
}

export type { IngestPanelProps };
