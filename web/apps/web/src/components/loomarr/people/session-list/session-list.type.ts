import type { SessionBody } from "@loomarr/api";

interface SessionListProps {
  userName: string;
  sessions: SessionBody[];
  // The session id currently being revoked, so only that row shows progress.
  revoking?: string;
  loading?: boolean;
  onRevoke?: (id: string) => void;
  onRevokeAll?: () => void;
  // Injectable clock (§5.2). Without it this component read Date.now() directly, so its
  // visual snapshots drifted as real time moved past the story's fixed timestamps —
  // a gate that fails on a Tuesday for no reason anyone can act on.
  now?: number;
  className?: string;
}

export type { SessionListProps };
