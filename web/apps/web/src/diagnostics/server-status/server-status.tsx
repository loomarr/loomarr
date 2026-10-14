import * as systemApi from "@loomarr/api/endpoints/system";
import { unwrap } from "@loomarr/api/unwrap";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { ActivityFeed } from "@/components/loomarr/dashboard/activity-feed";
import { ServiceControl } from "@/components/loomarr/dashboard/service-control";
import { ServicesPanel } from "@/components/loomarr/dashboard/services-panel";
import { useRestartWatchContext } from "@/dashboard/restart-watch-provider";

// ServerStatus — the connected services, recent activity and the restart control, moved here from
// the old Dashboard when Home replaced it (#1659, Q-H7: the operator panels keep working, on This
// server's routes). Admin-only, like the page that hosts it.
const ServerStatus = () => {
  const navigate = useNavigate();
  // ⚠ Services POLLS (30s); the feed does NOT (§12). A probe result only exists because the
  // server went and asked, so pushing it would mean probing forever on an idle install. A
  // feed row IS an event the server knows at write time, so it rides the `activity` frame —
  // wired centrally in @loomarr/core, which is why there is no refetchInterval here.
  const services = systemApi.useSystemServices({ query: { refetchInterval: 30_000 } });
  const activity = systemApi.useListActivity({ limit: 8 });
  // One cost query drives the confirm line, so it can never disagree with what a restart does.
  const restartCost = systemApi.useSystemRestartCost();
  const [serviceError, setServiceError] = useState<string | null>(null);
  // The SHARED watch (app shell). The overlay it drives covers the whole app, so an
  // operator who navigates away mid-restart still sees what is happening.
  const watch = useRestartWatchContext();
  const restart = systemApi.useSystemRestart({
    mutation: {
      onSuccess: () => {
        setServiceError(null);
        watch.begin();
      },
      onError: () => setServiceError("Couldn't restart. Loomarr is still running the old settings."),
    },
  });
  const cost = unwrap(restartCost.data);

  return (
    <div className="mb-6 flex flex-col gap-6 *:shrink-0">
      {services.data?.status === 200 ? (
        <ServicesPanel
          view={services.data.data}
          refreshing={services.isFetching}
          // "Fix →" routes to the settings that own the failing connection. A red dot
          // with nowhere to go is a puzzle, not a diagnosis (§12).
          onFix={(group) => navigate({ to: "/settings/connections", hash: group })}
          onDiagnose={(subsystem) =>
            navigate({
              to: "/settings/system/diagnostics",
              search: { view: "logs", level: "error", subsystem },
            })
          }
        />
      ) : null}
      {activity.data?.status === 200 ? <ActivityFeed entries={activity.data.data.activity ?? []} /> : null}
      {cost ? (
        <ServiceControl
          cost={cost}
          onRestart={() => restart.mutate()}
          restarting={watch.restarting}
          // A restart that never came back is the failure worth naming — it outranks
          // a request-level error, because the app is actually down.
          error={watch.failed ?? serviceError}
        />
      ) : null}
    </div>
  );
};

export { ServerStatus };
