import type { ChannelDTO } from "@loomarr/api";
import { channelsApi, toProblem, unwrap } from "@loomarr/api";
import { channelNumber } from "@loomarr/core";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link, Outlet, useLocation } from "@tanstack/react-router";
import { ArrowLeft } from "lucide-react";
import { toast } from "sonner";
import { useAuth } from "@/auth";
import { ChannelIdentityField } from "@/channels";
import type { OnAirState } from "@/components/loomarr";
import { ErrorState } from "@/components/loomarr";
import { NavTabs } from "@/components/ui";
import { useLoomarrEventListener } from "@/events";
import { useDocumentTitle } from "@/lib";
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

const SECTIONS = [
  { id: "info", label: "Channel info", to: "/channels/$id/info" },
  { id: "programming", label: "Programming", to: "/channels/$id/programming" },
  { id: "filler", label: "Filler", to: "/channels/$id/filler" },
  { id: "danger", label: "Danger zone", to: "/channels/$id/danger" },
] as const;

type AirState = { dot: OnAirState; label: string; detail: string };

const airStateOf = (ch: ChannelDTO): AirState => {
  const pushed = Boolean(ch.tunarrId);
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
  if (ch.status === "live" && pushed) {
    return { dot: "live", label: "On air", detail: "Playing now in your TV guide." };
  }
  // building, or live-without-a-push (not yet broadcasting). Once Tunarr is connected it
  // goes live on its own — no manual step.
  return {
    dot: "reconciling",
    label: "Not on air yet",
    detail: pushed ? "Getting ready to broadcast." : "It'll go live automatically once Tunarr is connected.",
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
  useDocumentTitle(unwrap(channel.data, (b) => b.name));

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
  };

  // Live update: a background reconcile (this edit, the sweep, a title landing) emits a
  // `channel` frame — refresh so the page reflects it with no manual action.
  useLoomarrEventListener({ onChannel: () => invalidate() });

  // Edits save seamlessly, then auto-reconcile server-side. A light toast confirms; the
  // edit is the trigger, never a separate "apply"/"rebuild" step.
  const update = channelsApi.useUpdateChannel({
    mutation: {
      onSuccess: () => {
        invalidate();
        toast.success("Saved");
      },
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't save that change"),
    },
  });
  const del = channelsApi.useDeleteChannel({
    mutation: {
      onSuccess: () => {
        toast.success("Channel deleted");
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
  if (!ch) return <p className="p-6 text-muted-foreground text-sm">Loading channel…</p>;

  const air = airStateOf(ch);

  // Count TITLES, not slots: slotCount includes commercial-break gaps (§10), so it would
  // read "12 of 15 shows ready" forever on a healthy channel with ad breaks. programCount +
  // pendingCount is the real title total; ready < total iff something is still acquiring.
  const ready = ch.programCount;
  const total = ch.programCount + ch.pendingCount;

  // The identity fields commit through mutateAsync so they can distinguish success (adopt +
  // close) from failure (a 409 renumber surfaces inline on the field, editor stays open). The
  // hook-level onError toast still fires; the field also gets the rejection to show in place.
  const saveName = (name: string | number) =>
    update.mutateAsync({ id, data: { name: String(name) } }).then(() => undefined);
  const saveNumber = (number: string | number) =>
    update.mutateAsync({ id, data: { number: Number(number) } }).then(() => undefined);
  // The icon is just another field on the same PATCH (§7) — not a bespoke endpoint — so it
  // commits exactly like the identity fields above. `""` clears it.
  const saveLogo = (logo: string) => update.mutateAsync({ id, data: { logo } }).then(() => undefined);

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-center gap-3 border-border border-b px-6 py-4">
        <Link
          to="/guide"
          aria-label="Back to channels"
          className="cursor-pointer text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="size-4" aria-hidden />
        </Link>
        {/* Identity. Admins edit the name/number inline with explicit Save/Cancel per field
            (ChannelIdentityField); viewers see the plain heading. The mono channel number is
            the anchor glyph either way (§2.2 — machine data is mono). */}
        {isAdmin ? (
          <div className="flex flex-1 flex-wrap items-center gap-3">
            <ChannelIdentityField
              label="Channel name"
              value={ch.name}
              variant="title"
              validate={(v) => (v.trim().length === 0 ? "Give the channel a name." : undefined)}
              onSave={saveName}
            />
            <label className="ml-auto flex items-center gap-2 text-muted-foreground text-sm">
              <span className="font-mono uppercase tracking-wide">Ch</span>
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
            </label>
          </div>
        ) : (
          <>
            <h1 className="font-semibold text-xl">{ch.name}</h1>
            <span className="ml-auto font-mono text-muted-foreground text-sm">
              Channel {channelNumber(ch.number)}
            </span>
          </>
        )}
      </header>

      {/* A HORIZONTAL tab bar under the header (admin only). Each tab is its OWN ROUTE now
          (V-nav-paths); a viewer sees no bar — just the read-only Channel info at /info. */}
      {isAdmin && (
        <div className="px-6 pt-2">
          <NavTabs
            label="Channel sections"
            linkComponent={Link}
            tabs={SECTIONS.map((s) => ({ id: s.id, label: s.label, to: s.to.replace("$id", id) }))}
            activeId={SECTIONS.find((s) => pathname.endsWith(`/${s.id}`))?.id ?? "info"}
          />
        </div>
      )}

      {/* One section at a time. The panel is centered at a comfortable width and scrolls if
          its own content is tall — but the page never grows into a long stack. */}
      <div className="flex-1 overflow-auto">
        <div className="mx-auto flex w-full max-w-4xl flex-col gap-4 p-6">
          <ChannelDetailProvider
            value={{
              id,
              channel: ch,
              isAdmin,
              air,
              ready,
              total,
              invalidate,
              saving: update.isPending,
              deleting: del.isPending,
              update: (data) => update.mutate({ id, data }),
              updateAsync: (data) => update.mutateAsync({ id, data }),
              savePolicy: (policy) => update.mutate({ id, data: { policy } }),
              // `strategy` is a CHANNEL field, not a policy one, so it takes its own PATCH —
              // but it is edited on the Programming surface beside Ordering, because that is
              // where its effect is visible: Ordering's "inherit channel default" has always
              // referred to this value.
              saveStrategy: (strategy) => update.mutate({ id, data: { strategy } }),
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

export { Route, SECTIONS };
