interface IngestPanelProps {
  // Whether this install can RUN the yt-dlp + ffmpeg tooling (§10, §16). The one feature
  // gate no setting can open, because no setting can assert that a binary executes.
  // ⚠ False no longer means "you chose the wrong image tag" — the single image always
  // ships the tooling — it means a degraded install, and the copy must say so.
  ingestAvailable: boolean;
  onIngested?: () => void;
  className?: string;
}

export type { IngestPanelProps };
