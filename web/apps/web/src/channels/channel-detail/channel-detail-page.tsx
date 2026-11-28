import * as channelsApi from "@loomarr/api/endpoints/channels";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { channelNumber } from "@loomarr/core/format";
import { Action, Card, Stack, Text } from "@loomarr/design-system";
import { ChannelIdent, NavTabs } from "@loomarr/ui";
import { Link, Outlet, useLocation } from "@tanstack/react-router";
import { useLayoutEffect, useRef } from "react";
import { toast } from "sonner";
import { useAuth } from "@/auth/use-auth";
import { ChannelIdentityField } from "@/channels/channel-identity-field";
import { ignoreFailure, useChannelSave, withPending } from "@/channels/use-channel-save";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { useBelowSm } from "@/lib/use-phone-width";
import { airStateOf, headerAirLine } from "./channel-air-state";
import { ChannelDetailProvider } from "./channel-detail-context";
import { useChannelDetailData } from "./use-channel-detail-data";

// Channel detail (§12). TWO AUDIENCES: the top answers a viewer's questions — is it on,
// what's playing, what's next, how full is it — in plain words. Editing (rename/renumber,
// the programming rules, pause/delete) is admin-only and lives inline (§7 PATCH). An edit
// auto-reconciles server-side and the page updates live via the `channel` SSE frame (§9
// self-maintaining); the one manual Build schedule is offered only when a live channel has
// nothing in the guide (#2728). The scheduler internals stay tucked in
// an admin-only "Advanced" disclosure so the viewer surface reads plainly.
//
// ⚠ Split into a layout + one route per section (V-nav-paths): `/channels/$id/info`,
// `/programming`, `/filler`, `/danger` — matching Settings. This file owns the mutators and the
// identity header; each section is its own route file reading them back through
// `useChannelDetail()` (`channel-detail-context.tsx`).

// `viewer: true` marks a section a member can reach too — the tab bar shows exactly those for a
// non-admin, and the full set for an admin. Watch is not a section: it is its own full-bleed route
// (`/watch/$id`, #2088), opened from the header's Watch button.
const SECTIONS = [
  { id: "info", label: "Channel info", to: "/channels/$id/info", viewer: true },
  { id: "programming", label: "Programming", to: "/channels/$id/programming", viewer: false },
  { id: "filler", label: "Filler", to: "/channels/$id/filler", viewer: false },
  { id: "danger", label: "Danger zone", to: "/channels/$id/danger", viewer: false },
] as const;

const ChannelDetailPage = ({ id, onDeleted }: { id: string; onDeleted: () => void }) => {
  const { isAdmin } = useAuth();
  const narrow = useBelowSm();
  // ⚠ From `useLocation`, not `?section=`: which panel is active is the PATH.
  const { pathname } = useLocation();
  // Every section renders in the one scroller below, so a new tab would open at the last tab's
  // offset (#2744). Each section starts at its own top.
  const sectionScroller = useRef<HTMLDivElement>(null);
  // biome-ignore lint/correctness/useExhaustiveDependencies: the path is the trigger, not a value read.
  useLayoutEffect(() => {
    if (sectionScroller.current) sectionScroller.current.scrollTop = 0;
  }, [pathname]);

  const {
    channel,
    mediaServerUrl,
    mediaServerName,
    nowNext: nowNextBody,
    timeline: timelineBody,
    nowMs,
    invalidate,
  } = useChannelDetailData(id, isAdmin);

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
        onDeleted();
      },
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't delete the channel"),
    },
  });

  if (channel.error) {
    return (
      <Stack padding={24}>
        <ErrorState error={channel.error} onRetry={() => channel.refetch()} />
      </Stack>
    );
  }
  const ch = unwrap(channel.data);
  // The route's loader ensured the channel, so it is here before this page mounts.
  if (!ch) return null;

  // The header state comes from now/next (#2728): a channel the guide has nothing for is not On air.
  const nowNext = nowNextBody?.channels.find((entry) => entry.channelId === id);
  const air = airStateOf(ch, nowNextBody ? Boolean(nowNext?.now || nowNext?.next) : undefined);
  const onNow = nowNext?.now;
  const airing = timelineBody?.airings.find(
    (a) => a.kind !== "filler" && a.startMs <= nowMs && nowMs < a.stopMs,
  );
  const airLine = headerAirLine(air, onNow, nowMs, airing);

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
    <Stack height="100%">
      {/* The web mock's header (#1659): back, the channel's icon (its monogram ident when it has
          none, N8), the name over an air line, and CH n at the right. The back link gets a 24 px
          target; the mock's 16 px glyph alone is under the minimum. */}
      <Stack
        align="center"
        borderBottomWidth={1}
        borderColor="borderDecorative"
        direction="row"
        gap="control"
        paddingHorizontal={{ base: 16, sm: 24 }}
        paddingVertical={16}
        render="header"
        wrap
      >
        {/* A 24 px ghost icon link; its 16 px glyph alone is under the minimum target. */}
        <Action
          accessibilityRole="link"
          compact
          icon="goBack"
          layout="icon"
          render={<Link to="/guide" />}
          style={{ marginLeft: -4 }}
          variant="ghost"
        >
          Back to channels
        </Action>
        {ch.logo ? (
          <Card
            borderRadius={8}
            flexShrink={0}
            height={48}
            level="elevated"
            overflow="hidden"
            position="relative"
            width={48}
          >
            <img src={ch.logo} alt="" style={{ height: "100%", objectFit: "cover", width: "100%" }} />
          </Card>
        ) : (
          <ChannelIdent name={ch.name} number={ch.number} size={48} />
        )}
        {/* On a phone this wrapper steps aside (`contents`): CH n joins the back link and icon on
            the first row, and the name, an input for an admin, takes a full-width row under them,
            so a long name fits instead of being cut inside its field (#1785). */}
        <Stack
          align="center"
          columnGap="3"
          direction="row"
          flexBasis="0%"
          flexGrow={1}
          flexShrink={1}
          minWidth={0}
          rowGap="1"
          style={narrow ? { display: "contents" } : undefined}
          wrap
        >
          <Stack
            flexBasis={narrow ? "100%" : "auto"}
            gap="0.5"
            minWidth={0}
            style={narrow ? { order: 1 } : undefined}
          >
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
              <Text as="h1" textRole="title">
                {ch.name}
              </Text>
            )}
            <Stack align="center" direction="row" gap="1.5" render="span">
              <Stack
                aria-hidden
                backgroundColor={airLine.onAir ? "$guideOnAir" : "$actionDisabled"}
                borderRadius={999}
                height={7}
                render="span"
                width={7}
              />
              <Text as="span" textRole="compact" tone="secondary">
                {airLine.text}
              </Text>
            </Stack>
          </Stack>
          <Stack align="center" direction="row" gap="inline" marginLeft="auto">
            <Text as="span" shout textRole="data">
              Ch
            </Text>
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
              <Stack paddingHorizontal={6} paddingVertical={2} render="span">
                <Text as="span" textRole="data" tone="primary">
                  {channelNumber(ch.number)}
                </Text>
              </Stack>
            )}
          </Stack>
        </Stack>
        {/* Watch opens the full-bleed player (#2088); there is no Watch tab. The media-server
            hand-off opens its front door in a new tab: Emby and Jellyfin expose no per-channel Live
            TV link, so the viewer goes on from there. */}
        <Stack align="center" direction="row" gap="2" wrap>
          {/* Action names its host a button unless told otherwise; these are links. */}
          <Action accessibilityRole="link" icon="play" render={<Link to="/watch/$id" params={{ id }} />}>
            Watch
          </Action>
          {mediaServerUrl && (
            <Action
              accessibilityRole="link"
              variant="outline"
              render={<a href={mediaServerUrl} target="_blank" rel="noopener noreferrer" />}
            >
              {`Open in ${mediaServerName}`}
            </Action>
          )}
        </Stack>
      </Stack>

      {/* A HORIZONTAL tab bar under the header. Each tab is its OWN ROUTE (V-nav-paths). An admin
          sees every section; a viewer's only section is Channel info, so they get no bar (Watch is
          the header's button, #2088). */}
      {(() => {
        const visible = SECTIONS.filter((s) => isAdmin || s.viewer);
        // One reachable section ⇒ no bar (a lone tab is chrome, not navigation).
        if (visible.length < 2) return null;
        // ⚠ No fallback tab. An admin's bar lights only the section the path names; lighting Channel
        // info for any other path (a `?? "info"` fallback) says they are on a page they aren't (#1987).
        return (
          // min-w-0 + overflow-x-auto: on a phone the four tabs scroll sideways inside this row
          // instead of widening the page past the viewport (#3404).
          <Stack
            data-testid="channel-tab-strip"
            flexShrink={1}
            minWidth={0}
            overflowX="auto"
            paddingHorizontal={{ base: 16, sm: 24 }}
            paddingTop={8}
          >
            <NavTabs
              label="Channel sections"
              linkComponent={Link}
              tabs={visible.map((s) => ({ id: s.id, label: s.label, to: s.to.replace("$id", id) }))}
              activeId={visible.find((s) => pathname.endsWith(`/${s.id}`))?.id}
            />
          </Stack>
        );
      })()}

      {/* One section at a time. The panel is centered at a comfortable width and scrolls if
          its own content is tall — but the page never grows into a long stack. */}
      {/* ⚠ `position: relative` is load-bearing. Visually-hidden text (`sr-only`) is absolutely
          positioned; with no positioned ancestor it is laid out against <body> at its natural
          flow position and stretches the document's scroll height, giving a second scrollbar
          behind this one. Making the scroller the containing block keeps it inside (#3404, the
          same bug as #2724 for Settings). */}
      <div
        data-testid="channel-section-scroller"
        ref={sectionScroller}
        style={{ flex: "1 1 0%", overflow: "auto", position: "relative" }}
      >
        <Stack gap="4" marginHorizontal="auto" maxWidth={896} padding={24} width="100%">
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
        </Stack>
      </div>
    </Stack>
  );
};

export { ChannelDetailPage };
