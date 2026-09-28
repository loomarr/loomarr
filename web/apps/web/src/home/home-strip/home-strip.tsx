import * as systemApi from "@loomarr/api/endpoints/system";
import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";
import { unwrap } from "@loomarr/api/unwrap";
import { Link, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { StatusStrip, StatusStripAction, type StatusStripItem } from "@/components/ui/status-strip";
import { useRestartWatchContext } from "@/dashboard/restart-watch-provider";
import { useNeedsYouCount } from "@/queue/use-requests";
import type { HomeStripProps } from "./home-strip.type";

const plural = (n: number, one: string, many: string) => (n === 1 ? one : many);

// The headline over every channel the guide read. A channel that is not live (paused, building,
// empty) is not "playing", so the sentence only claims "All" when every one is.
const playingTitle = (channels: readonly GuideChannelTimeline[]) => {
  const live = channels.filter((c) => c.status === "live").length;
  return live === channels.length
    ? `All ${live} ${plural(live, "channel is", "channels are")} playing`
    : `${live} of ${channels.length} channels are playing`;
};

// HomeStrip — the top of Home (#1659 web mock): one line saying how things are, then what needs
// someone. Admins get the operator items (services not answering, requests to decide, a pending
// restart); members get only their own requests that couldn't be built (Q-H1, Q-H2).
//
// The restart item appears only while a restart-scoped key is actually waiting (`pendingKeys`),
// which since #1695 is admin-only and names only what can't apply live.
const HomeStrip = ({ channels, loading, isAdmin }: HomeStripProps) => {
  const navigate = useNavigate();
  const needsYou = useNeedsYouCount();
  const services = systemApi.useSystemServices({ query: { enabled: isAdmin, refetchInterval: 30_000 } });
  const restartCost = systemApi.useSystemRestartCost({ query: { enabled: isAdmin } });
  const watch = useRestartWatchContext();
  const [confirming, setConfirming] = useState(false);
  const restart = systemApi.useSystemRestart({
    mutation: {
      onSuccess: () => {
        setConfirming(false);
        watch.begin();
      },
    },
  });

  const empty = !loading && channels.length === 0;
  const failing = isAdmin ? (unwrap(services.data, (v) => v.rows)?.filter((r) => !r.ok) ?? []) : [];
  const pendingKeys = isAdmin ? (unwrap(restartCost.data, (c) => c.pendingKeys) ?? []) : [];

  const items: StatusStripItem[] = [];
  if (!loading && !empty) {
    if (failing.length > 0) {
      items.push({
        id: "services",
        tone: "error",
        title: `${failing.length} connected ${plural(failing.length, "service isn’t", "services aren’t")} answering`,
        action: (
          <StatusStripAction
            primary
            onClick={() => navigate({ to: "/settings/connections", hash: failing[0]?.settingsGroup })}
          >
            Fix
          </StatusStripAction>
        ),
      });
    }
    if (isAdmin && needsYou > 0) {
      items.push({
        id: "requests",
        tone: "attention",
        title: `${needsYou} ${plural(needsYou, "request needs", "requests need")} you`,
        action: (
          <StatusStripAction primary render={<Link to="/requests/needs-you" />}>
            Review
          </StatusStripAction>
        ),
      });
    }
    if (!isAdmin && needsYou > 0) {
      items.push({
        id: "failed",
        tone: "error",
        title: `${needsYou} of your requests couldn’t be built`,
        action: (
          <StatusStripAction render={<Link to="/requests/needs-you" />}>Edit and retry</StatusStripAction>
        ),
      });
    }
    if (pendingKeys.length > 0) {
      items.push({
        id: "restart",
        tone: "notice",
        title: `Restart Loomarr to finish saving ${pendingKeys.length} ${plural(pendingKeys.length, "setting", "settings")}`,
        action: <StatusStripAction onClick={() => setConfirming(true)}>Restart…</StatusStripAction>,
      });
    }
  }

  const title = loading
    ? "Checking your channels…"
    : empty
      ? "Nothing on air yet"
      : !isAdmin
        ? "Everything’s playing"
        : failing.length > 0
          ? "Something needs fixing"
          : playingTitle(channels);

  return (
    <div className="flex flex-col gap-2">
      <StatusStrip
        tone={loading || empty ? "idle" : isAdmin && failing.length > 0 ? "error" : "ok"}
        loading={loading}
        title={title}
        items={items}
        action={
          empty ? (
            <Button size="sm" render={<Link to="/guide" search={{ new: "1" }} />}>
              {isAdmin ? "Add your first channel" : "Request a channel"}
            </Button>
          ) : undefined
        }
      />
      {confirming && (
        <div className="flex items-center gap-2 rounded-lg border border-signal/35 bg-static-900 px-4 py-2.5">
          <p className="m-0 min-w-0 flex-1 text-[13px]">
            {/* A restart that never came back outranks a request error: the app is actually down. */}
            {(watch.failed ?? restart.isError)
              ? "Couldn't restart. Loomarr is still running the old settings."
              : "Restart now? Channels pause for a few seconds, then come back on their own."}
          </p>
          <Button size="sm" disabled={restart.isPending || watch.restarting} onClick={() => restart.mutate()}>
            Restart now
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setConfirming(false)}>
            Cancel
          </Button>
        </div>
      )}
    </div>
  );
};

export { HomeStrip };
