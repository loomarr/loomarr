import { createFileRoute } from "@tanstack/react-router";
import { useAuth } from "@/auth/use-auth";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { DEFAULT_APPLICATION_FILTERS, DiagnosticsPage, type DiagnosticsSearch } from "@/diagnostics";
import { DiagnosticsPrototype, type PrototypeVariant } from "@/diagnostics/diagnostics-prototype";
import { PlayoutDiagnostics } from "@/diagnostics/playout-diagnostics";

const oneOf = <T extends string>(value: unknown, allowed: readonly T[], fallback: T): T =>
  typeof value === "string" && allowed.includes(value as T) ? (value as T) : fallback;

type DiagnosticsRouteSearch = Partial<DiagnosticsSearch> & { variant?: PrototypeVariant };

const validateSearch = (raw: Record<string, unknown>): DiagnosticsRouteSearch => ({
  ...(raw.variant === "A" || raw.variant === "B" || raw.variant === "C" ? { variant: raw.variant } : {}),
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
  processRange: oneOf(raw.processRange, ["1h", "6h", "24h"] as const, "1h"),
  processStatus: oneOf(
    raw.processStatus,
    ["all", "running", "succeeded", "failed", "cancelled"] as const,
    "all",
  ),
  processPurpose: typeof raw.processPurpose === "string" ? raw.processPurpose.slice(0, 128) : "",
  processChannelId: typeof raw.processChannelId === "string" ? raw.processChannelId.slice(0, 128) : "",
  processJobId: typeof raw.processJobId === "string" ? raw.processJobId.slice(0, 128) : "",
});

const DiagnosticsRoute = () => {
  const { isAdmin } = useAuth();
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const { variant, ...diagnosticsSearch } = search;
  const normalized: DiagnosticsSearch = {
    view: search.view ?? "health",
    processRange: "1h",
    processStatus: "all",
    processPurpose: "",
    processChannelId: "",
    processJobId: "",
    ...DEFAULT_APPLICATION_FILTERS,
    ...diagnosticsSearch,
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
  if (import.meta.env.DEV && variant) {
    return (
      <DiagnosticsPrototype
        variant={variant}
        showSwitcher
        onVariantChange={(next) => void navigate({ search: { ...search, variant: next }, replace: true })}
      />
    );
  }
  return (
    <DiagnosticsPage
      search={normalized}
      onSearchChange={(next) => void navigate({ search: next, replace: true })}
      onOpenRelated={(kind, id) => {
        if (kind === "channel") {
          void navigate({ to: "/channels/$id", params: { id } });
        } else {
          void navigate({ to: "/settings/system/tasks" });
        }
      }}
      playout={
        <PlayoutDiagnostics
          filters={normalized}
          onFiltersChange={(next) => void navigate({ search: next, replace: true })}
          onOpenChannel={(id) => void navigate({ to: "/channels/$id", params: { id } })}
        />
      }
    />
  );
};

const Route = createFileRoute("/_authed/settings/system/diagnostics")({
  validateSearch,
  component: DiagnosticsRoute,
});

export { Route };
