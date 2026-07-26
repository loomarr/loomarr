interface NowNext {
  now?: { title: string; until?: string };
  next?: { title: string };
  gap?: boolean; // a flex / commercial-pod gap is currently airing
}

interface NowNextStripProps extends NowNext {
  className?: string;
}

export type { NowNext, NowNextStripProps };
