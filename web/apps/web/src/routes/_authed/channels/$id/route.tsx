import * as channelsApi from "@loomarr/api/endpoints/channels";
import * as settingsApi from "@loomarr/api/endpoints/settings";
import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { NowNextEntry } from "@loomarr/api/models/nowNextEntry";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { channelNumber } from "@loomarr/core/format";
import { ChannelIdent } from "@loomarr/ui";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link, Outlet, useLocation } from "@tanstack/react-router";
import { ArrowLeft, Play } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";
import { useAuth } from "@/auth/use-auth";
import { ChannelIdentityField } from "@/channels/channel-identity-field";
import { ignoreFailure, useChannelSave, withPending } from "@/channels/use-channel-save";
import type { OnAirState } from "@/components/loomarr/channels/on-air-indicator";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Button } from "@/components/ui/button";
import { NavTabs } from "@/components/ui/nav-tabs";
import { useLoomarrEventListener } from "@/events/events-provider";
import { useDocumentTitle } from "@/lib/use-document-title";
import { cn } from "@/lib/utils";
import { ChannelDetailProvider } from "./-channel-detail-context";

// Channel detail (§12). TWO AUDIENCES: the top answers a viewer's questions — is it on,
// what's playing, what's next, how full is it — in plain words. Editing (rename/renumber,
// the programming rules, pause/delete) is admin-only and lives inline (§7 PATCH). There is
// NO manual "rebuild" button: an edit auto-reconciles server-side and the page updates live
// via the `channel` SSE frame (§9 self-maintaining). The scheduler internals stay tucked in
// an admin-only "Advanced" disclosure so the viewer surface reads plainly.
//
// ⚠ Split into a layout + one route per section (V-nav-paths): `/channels/$id/info`,
// `/programming`, `/filler`, `/danger` — matching Settings. This file owns the channel fetch,
// the mutators, and the identity header; each section is its own route file reading them back
// through `useChannelDetail()` (`-channel-detail-context.tsx`).

// `viewer: true` marks a section a member can reach too — the tab bar shows exactly those for a
// non-admin, and the full set for an admin. Watch is not a section: it is its own full-bleed route
// (`/watch/$id`, #2088), opened from the header's Watch button.
const SECTIONS = [
  { id: "info", label: "Channel info", to: "/channels/$id/info", viewer: true },
  { id: "programming", label: "Programming", to: "/channels/$id/programming", viewer: false },
  { id: "filler", label: "Filler", to: "/channels/$id/filler", viewer: false },
  { id: "danger", label: "Danger zone", to: "/channels/$id/danger", viewer: false },
] as const;

const FLAVOR_NAMES: Record<string, string> = { emby: "Emby", jellyfin: "Jellyfin" };

type AirState = { dot: OnAirState; label: string; detail: string };

// How often the header's minutes-left moves on.
const NOW_TICK_MS = 30_000;

// headerAirLine is the line under the name (the web mock): a red dot and "On now: <title> · Nm
// left" while a programme airs, a grey dot and "Paused" for a paused channel. The mock draws only
// those two; any other state shows the page's own air label (Off air, Updating…) on the grey dot.
const headerAirLine = (
  air: AirState,
  now: NowNextEntry | undefined,
  nowMs: number,
): { onAir: boolean; text: string } => {
  if (air.dot === "live" && now && !now.gap && now.startMs <= nowMs && nowMs < now.stopMs) {
    const left = Math.max(1, Math.round((now.stopMs - nowMs) / 60_000));
    return { onAir: true, text: `On now: ${now.title} · ${left}m left` };
  }
  return { onAir: false, text: air.label };
};

const airStateOf = (ch: ChannelDTO): AirState => {
  // "Broadcasting" is backend-agnostic. Tunarr channels signal it with a pushed projection
  // (`tunarrId`); internal-playout channels — the DEFAULT backend, which never gets a
  // `tunarrId` — signal it with `inAppPlayable`, the server-resolved surfability truth
  // (§9.1 V57: effective backend plays internally AND the lifecycle is reconcilable). Keying
  // "On air" on `tunarrId` alone told every working internal-playout channel it was "Not on
  // air yet — connect Tunarr", contradicting reality at the first-channel "aha" moment.
  const pushed = Boolean(ch.tunarrId);
  const broadcasting = pushed || ch.inAppPlayable;
  if (ch.status === "detached") {
    return { dot: "off", label: "Off air", detail: "Loomarr no longer manages this channel." };
  }
  if (ch.status === "paused") {
    return {
      dot: "off",
      label: "Paused",
      detail: "Off air, but kept. Resume it below whenever you like.",
    };
  }
  if (ch.status === "drifted") {
    return {
      dot: "reconciling",
      label: "On air (catching up)",
      detail: "Something changed in your library. Loomarr is bringing the lineup back in line.",
    };
  }
  if (ch.status === "live" && broadcasting) {
    return { dot: "live", label: "On air", detail: "Playing now in your TV guide." };
  }
  // building, or live-without-broadcasting (not yet reaching a viewer). An internal-playout
  // channel gets there on its own as it reconciles; a Tunarr-backed one once Tunarr connects.
  return {
    dot: "reconciling",
    label: "Not on air yet",
    detail: pushed
      ? "Getting ready to broadcast."
      : "It'll go live automatically once it's ready to broadcast.",
  };
};

const ChannelDetailLayout = () => {
  const { id } = Route.useParams();
  const navigate = Route.useNavigate();
  const { isAdmin } = useAuth();
  const queryClient = useQueryClient();
  // ⚠ From `useLocation`, not `?section=`: which panel is active is now the PATH.
  const { pathname } = useLocation();

  const channel = channelsApi.useGetChannel(id);
  // The "Open in …" hand-off names and opens the configured media server. `library.flavor` gives the
  // name (Emby/Jellyfin) and `library.url` its front door, both non-secret settings. An unset URL
  // hides the link rather than leaving it doing nothing. The settings list is admin-only, so a
  // member doesn't ask for it and gets no link.
  const settingsList = settingsApi.useSettingsList({ query: { enabled: isAdmin, retry: false } });
  const settings = unwrap(settingsList.data, (b) => b.settings) ?? [];
  const mediaServerUrl = settings.find((e) => e.key === "library.url")?.value || undefined;
  const mediaServerName =
    FLAVOR_NAMES[settings.find((e) => e.key === "library.flavor")?.value ?? ""] ?? "your media server";
  useDocumentTitle(unwrap(channel.data, (b) => b.name));
  // The header's "On now: … · Nm left" reads the shared now/next list; the minutes move on a tick.
  const nowNextBody = unwrap(channelsApi.useChannelsNowNext({ query: { staleTime: 15_000 } }).data);
  const [nowMs, setNowMs] = useState(() => Date.now());
  useEffect(() => {
    const tick = setInterval(() => setNowMs(Date.now()), NOW_TICK_MS);
    return () => clearInterval(tick);
  }, []);

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: channelsApi.getGetChannelQueryKey(id) });
    void queryClient.invalidateQueries({ queryKey: channelsApi.getChannelsNowNextQueryKey() });
    // The Overview's "what's on later" strip reads its own per-channel guide endpoint, so a
    // reconcile that reshuffles the lineup must refresh it too — otherwise the strip shows a
    // stale timeline until a manual reload (P7 gate: the `channel` frame refreshes the strip).
    void queryClient.invalidateQueries({ queryKey: channelsApi.getChannelUpcomingQueryKey(id) });
    // The saved pod preview (GET …/pods) lives under its own key, not the channel key, so
    // a reconcile that changes the break — including an applied filler edit — wouldn't
    // refresh it without this. The Filler section reads the saved break here.
    void queryClient.invalidateQueries({ queryKey: channelsApi.getPreviewChannelPodsQueryKey(id) });
    // The saved cycle preview also sizes one pass for the Programming page's play-order line, so
    // a changed lineup, scope or order must refresh it too.
    void queryClient.invalidateQueries({ queryKey: channelsApi.getPreviewChannelCycleQueryKey(id) });
  };

  // Live update: a background reconcile (this edit, the sweep, a title landing) emits a
  // `channel` frame — refresh so the page reflects it with no manual action.
  useLoomarrEventListener({ onChannel: () => invalidate() });

  // Every edit on this page, from every section, saves through the channel's one serial queue
  // (#1895, #1899): editors that each sent their own PATCH raced on the channel revision and lost
  // whichever landed second. Edits save seamlessly, then auto-reconcile server-side; a light toast
  // confirms, and the edit is the trigger, never a separate "apply"/"rebuild" step.
  const channelSave = useChannelSave(id);
  const del = channelsApi.useDeleteChannel({
    mutation: {
      onSuccess: (_response, variables) => {
        const purged = variables.params?.purge === true;
        toast.success(purged ? "Channel deleted from Loomarr and Tunarr" : "Stopped managing channel", {
          description: purged
            ? "Its Loomarr record and any retained Tunarr channel were removed."
            : "Loomarr kept its record and left any Tunarr channel in place.",
        });
        void navigate({ to: "/guide" });
      },
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't delete the channel"),
    },
  });

  if (channel.error) {
    return (
      <div className="p-6">
        <ErrorState error={channel.error} onRetry={() => channel.refetch()} />
      </div>
    );
  }
  const ch = unwrap(channel.data);
  if (!ch) return <p className="text-muted-foreground p-6 text-sm">Loading channel…</p>;

  const air = airStateOf(ch);
  const onNow = nowNextBody?.channels.find((entry) => entry.channelId === id)?.now;
  const airLine = headerAirLine(air, onNow, nowMs);

  // Count TITLES, not slots: slotCount includes commercial-break gaps (§10), so it would
  // read "12 of 15 shows ready" forever on a healthy channel with ad breaks. programCount +
  // pendingCount is the real title total; ready < total iff something is still acquiring.
  const ready = ch.programCount;
  const total = ch.programCount + ch.pendingCount;

  // The identity fields await the save so they can distinguish success (adopt + close) from
  // failure (a 409 renumber surfaces inline on the field, editor stays open). The writer's error
  // toast still fires; the field also gets the rejection to show in place.
  const saveName = (name: string | number) => channelSave.save({ name: String(name) });
  const saveNumber = (number: string | number) => channelSave.save({ number: Number(number) });
  // The icon is just another field on the same PATCH (§7) — not a bespoke endpoint — so it
  // commits exactly like the identity fields above. `""` clears it.
  const saveLogo = (logo: string) => channelSave.save({ logo });

  return (
    <div className="flex h-full flex-col">
      {/* The web mock's header (#1659): back, the channel's icon (its monogram ident when it has
          none, N8), the name over an air line, and CH n at the right. The back link gets a 24 px
          target; the mock's 16 px glyph alone is under the minimum. */}
      <header className="border-border flex flex-wrap items-center gap-3 border-b px-4 py-4 sm:px-6">
        <Link
          to="/guide"
          aria-label="Back to channels"
          className="text-muted-foreground hover:text-foreground -ml-1 flex size-6 cursor-pointer items-center justify-center rounded"
        >
          <ArrowLeft className="size-4" aria-hidden />
        </Link>
        {ch.logo ? (
          <div className="border-border bg-static-800 relative size-12 shrink-0 overflow-hidden rounded-lg border">
            <img src={ch.logo} alt="" className="size-full object-cover" />
          </div>
        ) : (
          <ChannelIdent name={ch.name} number={ch.number} size={48} />
        )}
        {/* On a phone this wrapper steps aside (`contents`): CH n joins the back link and icon on
            the first row, and the name, an input for an admin, takes a full-width row under them,
            so a long name fits instead of being cut inside its field (#1785). */}
        <div className="flex min-w-0 flex-1 flex-wrap items-center gap-x-3 gap-y-1 max-sm:contents">
          <div className="flex min-w-0 basis-full flex-col gap-0.5 max-sm:order-last sm:basis-auto">
            {/* Admins still rename and renumber here with Save/Cancel per field, until the
                maintainer's click-to-edit mock lands; everyone else sees the mock's text. */}
            {isAdmin ? (
              <ChannelIdentityField
                label="Channel name"
                value={ch.name}
                variant="title"
                validate={(v) => (v.trim().length === 0 ? "Give the channel a name." : undefined)}
                onSave={saveName}
              />
            ) : (
              <h1 className="text-xl leading-tight font-semibold">{ch.name}</h1>
            )}
            <span className="text-muted-foreground flex items-center gap-1.5 text-sm">
              <span
                aria-hidden
                className={cn("size-[7px] rounded-full", airLine.onAir ? "bg-onair" : "bg-static-500")}
              />
              {airLine.text}
            </span>
          </div>
          <div className="text-muted-foreground ml-auto flex items-center gap-2 text-sm">
            <span className="font-mono tracking-wide uppercase">Ch</span>
            {isAdmin ? (
              <ChannelIdentityField
                label="Channel number"
                value={ch.number}
                type="number"
                variant="compact"
                validate={(v) => {
                  const n = Number(v);
                  if (v.trim() === "" || !Number.isInteger(n) || n < 1) return "Use a whole number ≥ 1.";
                  return undefined;
                }}
                onSave={saveNumber}
              />
            ) : (
              <span className="text-foreground px-1.5 py-0.5 font-mono">{channelNumber(ch.number)}</span>
            )}
          </div>
        </div>
        {/* Watch opens the full-bleed player (#2088); it replaced the Watch tab. The media-server
            hand-off opens its front door in a new tab: Emby and Jellyfin expose no per-channel Live
            TV link, so the viewer goes on from there. */}
        <div className="flex w-full items-center gap-2 sm:w-auto">
          <Button render={<Link to="/watch/$id" params={{ id }} />}>
            <Play className="size-4 fill-current" aria-hidden />
            Watch
          </Button>
          {mediaServerUrl && (
            <Button
              variant="outline"
              render={<a href={mediaServerUrl} target="_blank" rel="noopener noreferrer" />}
            >
              Open in {mediaServerName}
            </Button>
          )}
        </div>
      </header>

      {/* A HORIZONTAL tab bar under the header. Each tab is its OWN ROUTE (V-nav-paths). An admin
          sees every section; a viewer's only section is Channel info, so they get no bar (Watch is
          the header's button, #2088). */}
      {(() => {
        const visible = SECTIONS.filter((s) => isAdmin || s.viewer);
        // One reachable section ⇒ no bar (a lone tab is chrome, not navigation).
        if (visible.length < 2) return null;
        // ⚠ No fallback tab. An admin's bar lights only the section the path names; lighting Channel
        // info for any other path (the old `?? "info"`) said they were on a page they weren't (#1987).
        return (
          <div className="px-4 pt-2 sm:px-6">
            <NavTabs
              label="Channel sections"
              linkComponent={Link}
              tabs={visible.map((s) => ({ id: s.id, label: s.label, to: s.to.replace("$id", id) }))}
              activeId={visible.find((s) => pathname.endsWith(`/${s.id}`))?.id}
            />
          </div>
        );
      })()}

      {/* One section at a time. The panel is centered at a comfortable width and scrolls if
          its own content is tall — but the page never grows into a long stack. */}
      <div className="flex-1 overflow-auto">
        <div className="max-w-detail mx-auto flex w-full flex-col gap-4 p-6">
          <ChannelDetailProvider
            value={{
              id,
              channel: withPending(ch, channelSave.pending),
              isAdmin,
              air,
              ready,
              total,
              invalidate,
              saving: channelSave.isSaving,
              deleting: del.isPending,
              save: (edit) => void channelSave.save(edit).catch(ignoreFailure),
              savePolicy: (policy) => void channelSave.save({ policy }).catch(ignoreFailure),
              saveLogo,
              onDelete: ({ purge }) => del.mutate({ id, params: { purge } }),
            }}
          >
            <Outlet />
          </ChannelDetailProvider>
        </div>
      </div>
    </div>
  );
};

const Route = createFileRoute("/_authed/channels/$id")({
  component: ChannelDetailLayout,
});

export { airStateOf, Route, SECTIONS };
