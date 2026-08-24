import { createFileRoute } from "@tanstack/react-router";
import { useAuth } from "@/auth/use-auth";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { DEFAULT_APPLICATION_FILTERS, DiagnosticsPage, type DiagnosticsSearch } from "@/diagnostics";

const oneOf = <T extends string>(value: unknown, allowed: readonly T[], fallback: T): T =>
  typeof value === "string" && allowed.includes(value as T) ? (value as T) : fallback;

const validateSearch = (raw: Record<string, unknown>): Partial<DiagnosticsSearch> => ({
  view: oneOf(raw.view, ["health", "application", "playout"] as const, "health"),
  range: oneOf(raw.range, ["1h", "6h", "24h"] as const, DEFAULT_APPLICATION_FILTERS.range),
  level: oneOf(
    raw.level,
    ["all", "debug", "info", "warn", "error"] as const,
    DEFAULT_APPLICATION_FILTERS.level,
  ),
  source: oneOf(
    raw.source,
    ["all", "server", "web", "android_tv"] as const,
    DEFAULT_APPLICATION_FILTERS.source,
  ),
  subsystem: typeof raw.subsystem === "string" ? raw.subsystem.slice(0, 128) : "",
  text: typeof raw.text === "string" ? raw.text.slice(0, 256) : "",
  ...(typeof raw.requestId === "string" ? { requestId: raw.requestId.slice(0, 128) } : {}),
  ...(typeof raw.playbackSessionId === "string"
    ? { playbackSessionId: raw.playbackSessionId.slice(0, 128) }
    : {}),
  ...(typeof raw.channelId === "string" ? { channelId: raw.channelId.slice(0, 128) } : {}),
  ...(typeof raw.scheduleBlockId === "string" ? { scheduleBlockId: raw.scheduleBlockId.slice(0, 128) } : {}),
  ...(typeof raw.jobId === "string" ? { jobId: raw.jobId.slice(0, 128) } : {}),
  ...(typeof raw.processRunId === "string" ? { processRunId: raw.processRunId.slice(0, 128) } : {}),
  ...(typeof raw.processId === "string" ? { processId: raw.processId.slice(0, 128) } : {}),
});

const DiagnosticsRoute = () => {
  const { isAdmin } = useAuth();
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const normalized: DiagnosticsSearch = {
    view: search.view ?? "health",
    ...DEFAULT_APPLICATION_FILTERS,
    ...search,
  };
  if (!isAdmin) {
    return (
      <div className="p-6">
        <EmptyState
          title="Diagnostics are for admins"
          description="This surface contains machine state and technical logs."
        />
      </div>
    );
  }
  return (
    <DiagnosticsPage
      search={normalized}
      onSearchChange={(next) => void navigate({ search: next, replace: true })}
      playout={
        <p className="rounded-lg border border-border bg-card px-4 py-8 text-center text-muted-foreground text-sm">
          Loading Process-run diagnostics…
        </p>
      }
    />
  );
};

const Route = createFileRoute("/_authed/settings/system/diagnostics")({
  validateSearch,
  component: DiagnosticsRoute,
});

export { Route };
