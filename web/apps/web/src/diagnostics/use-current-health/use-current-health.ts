import * as diagnosticsApi from "@loomarr/api/endpoints/diagnostics";
import { useEffect } from "react";

// useCurrentHealth reads GET /v1/diagnostics/health, which its `health` frame keeps current. A
// continuous check also goes stale when its last result runs out, which no frame announces, so
// the read is repeated at the report's nextRefreshAt, the earliest time that can happen (#3352).
const useCurrentHealth = ({ enabled = true }: { enabled?: boolean } = {}) => {
  const query = diagnosticsApi.useGetCurrentHealth({ query: { enabled, retry: false } });
  const nextRefreshAt = query.data?.status === 200 ? query.data.data.nextRefreshAt : 0;
  const { refetch } = query;
  useEffect(() => {
    if (!enabled || !nextRefreshAt) return;
    // A moment past the deadline, so the server agrees it has passed.
    const timer = setTimeout(() => void refetch(), Math.max(0, nextRefreshAt - Date.now()) + 1_000);
    return () => clearTimeout(timer);
  }, [enabled, nextRefreshAt, refetch]);
  return query;
};

export { useCurrentHealth };
