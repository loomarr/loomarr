import * as diagnosticsApi from "@loomarr/api/endpoints/diagnostics";
import { useRefetchAt } from "@/lib/use-refetch-at";

// useCurrentHealth reads GET /v1/diagnostics/health, which its `health` frame keeps current. A
// continuous check also goes stale when its last result runs out, which no frame announces, so
// the read is repeated at the report's nextRefreshAt, the soonest that can happen (#3352).
const useCurrentHealth = ({ enabled = true }: { enabled?: boolean } = {}) => {
  const query = diagnosticsApi.useGetCurrentHealth({ query: { enabled, retry: false } });
  useRefetchAt({
    deadline: query.data?.status === 200 ? query.data.data.nextRefreshAt : undefined,
    answeredAt: query.dataUpdatedAt,
    refetch: query.refetch,
    enabled,
  });
  return query;
};

export { useCurrentHealth };
