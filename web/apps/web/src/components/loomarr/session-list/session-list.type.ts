import type { SessionBody } from "@loomarr/api";

interface SessionListProps {
  userName: string;
  sessions: SessionBody[];
  // The session id currently being revoked, so only that row shows progress.
  revoking?: string;
  loading?: boolean;
  onRevoke?: (id: string) => void;
  onRevokeAll?: () => void;
  className?: string;
}

export type { SessionListProps };
