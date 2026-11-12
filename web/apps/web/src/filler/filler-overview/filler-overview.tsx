import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { FillerReadinessDTO } from "@loomarr/api/models/fillerReadinessDTO";
import type { PoolChannelDTO } from "@loomarr/api/models/poolChannelDTO";
import type { PoolChannelDTOLevel } from "@loomarr/api/models/poolChannelDTOLevel";
import { unwrap } from "@loomarr/api/unwrap";
import { formatDuration, pluralize } from "@loomarr/core/format";
import { Action, Card, Stack, StatusLabel, type StatusTone, Text } from "@loomarr/design-system";
import { Link } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { useAuth } from "@/auth/use-auth";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import type { FillerSettingsSection } from "../filler-settings-section";

type ActionBase = {
  title: string;
  description: string;
  label: string;
};

type ReadinessAction = ActionBase &
  (
    | {
        to: "/filler/settings/$section";
        section: FillerSettingsSection;
      }
    | {
        to: "/filler/incoming" | "/filler/library" | "/filler/sources";
        section?: never;
      }
  );

// This maps the readiness projection's server-owned action enum to presentation only. Priority
// and whole-workspace health are never reconstructed from subsystem counts in the browser (§10).
const readinessAction = (readiness: FillerReadinessDTO): ReadinessAction | undefined => {
  switch (readiness.nextAction) {
    case "none":
      return undefined;
    case "enable_fetch":
      return {
        title: "Turn on automatic sourcing",
        description: "Automatic source checks are off, so Loomarr cannot keep the filler catalog supplied.",
        label: "Review automation",
        to: "/filler/settings/$section",
        section: "downloads",
      };
    case "free_catalog_capacity":
      return {
        title: "Make room in the filler catalog",
        description: `Automatic sourcing paused at ${pluralize(readiness.actionCount ?? 0, "catalog clip")}. Remove clips or raise the limit to resume it.`,
        label: "Review limits",
        to: "/filler/settings/$section",
        section: "storage",
      };
    case "free_disposable_space":
      return {
        title: "This drive needs room",
        description:
          "Loomarr paused new filler before it used space kept for this device. Check for old temporary downloads or choose what to do next.",
        label: "Review storage",
        to: "/filler/sources",
      };
    case "choose_another_folder":
      return {
        title: "Loomarr cannot check this drive",
        description: "Choose a filler folder on a drive whose free space Loomarr can measure safely.",
        label: "Choose folder",
        to: "/filler/settings/$section",
        section: "folders",
      };
    case "change_storage_limit":
      return {
        title: "Filler reached its allowance",
        description: "Remove clips or give the filler library more room to resume automatic sourcing.",
        label: "Review storage",
        to: "/filler/settings/$section",
        section: "storage",
      };
    case "retry_acquisition":
      return {
        title: "A download needs another try",
        description: `${pluralize(readiness.actionCount ?? 0, "item")} could not finish downloading.`,
        label: "Open Incoming",
        to: "/filler/incoming",
      };
    case "retry_failed_work":
      return {
        title: "Some filler can be retried",
        description: `${pluralize(readiness.actionCount ?? 0, "item")} hit a recoverable processing problem.`,
        label: "Open Incoming",
        to: "/filler/incoming",
      };
    case "review_incoming":
      return {
        title: "A few clips need your help",
        description: `${pluralize(readiness.actionCount ?? 0, "clip")} ${readiness.actionCount === 1 ? "needs" : "need"} a choice before preparation can continue.`,
        label: "Review clips",
        to: "/filler/incoming",
      };
    case "add_filler":
      return {
        title: "Add filler to get started",
        description: "No playable filler is available yet. Add a source or drop in your own clips.",
        label: "Open sources",
        to: "/filler/sources",
      };
    case "improve_channel_coverage": {
      const channel = readiness.pool.channels.find(
        (candidate) => candidate.channelId === readiness.channelId,
      );
      return {
        title: "A channel needs better filler coverage",
        description: channel
          ? `${channel.number} · ${channel.name} has a playable pool, but its coverage is still thin.`
          : "At least one channel has a playable pool, but its coverage is still thin.",
        label: "Browse library",
        to: "/filler/library",
      };
    }
    default: {
      const unhandled: never = readiness.nextAction;
      return unhandled;
    }
  }
};

// A coverage card's level badge. The words are the web mock's ("Good match", "Years loosened",
// "Bumpers only"; it draws no audience-only card, so that one borrows the coverage meter's
// label). The tones are NOT the mock's: it painted the worst rung green and the best amber, and
// the redesign map (#1659) corrects that so green means good and red means nothing to play.
// Keyed by the generated union, so a new rung is a compile error here.
const COVERAGE_BADGE: Record<PoolChannelDTOLevel, { label: string; tone: StatusTone }> = {
  exact: { label: "Good match", tone: "ok" },
  widened: { label: "Years loosened", tone: "warn" },
  audience: { label: "Audience only", tone: "warn" },
  bumper_card: { label: "Bumpers only", tone: "error" },
};

// The numbers a coverage card shows. Channels whose numbers all match share one pool.
const coverageKey = (channel: PoolChannelDTO) =>
  [channel.level, channel.durationMs, channel.total, channel.categories, channel.brands].join("|");

const sharedPool = (channels: PoolChannelDTO[]): PoolChannelDTO | undefined => {
  const [first] = channels;
  return first &&
    channels.length > 1 &&
    channels.every((channel) => coverageKey(channel) === coverageKey(first))
    ? first
    : undefined;
};

const CoverageCard = ({ coverage, title }: { coverage: PoolChannelDTO; title: ReactNode }) => {
  const badge = COVERAGE_BADGE[coverage.level];
  return (
    <Card padding={16}>
      <Stack align="start" direction="row" gap="control" justify="between">
        <div className="min-w-0">
          {title}
          <p className="text-muted-foreground mt-1 text-sm">
            {coverage.level === "bumper_card"
              ? "Only a bumper card"
              : `${formatDuration(coverage.durationMs)} playable · ${pluralize(coverage.total, "clip")}`}
          </p>
        </div>
        <StatusLabel tone={badge.tone}>{badge.label}</StatusLabel>
      </Stack>
      <p className="text-muted-foreground mt-3 text-xs">
        {coverage.categories} {coverage.categories === 1 ? "category" : "categories"} ·{" "}
        {pluralize(coverage.brands, "brand")}
      </p>
    </Card>
  );
};

const FillerOverview = () => {
  const { isAdmin } = useAuth();
  const readinessQuery = fillerApi.useFillerReadiness();
  const readiness = unwrap(readinessQuery.data, (body) => body);

  if (readinessQuery.error) {
    return <ErrorState error={readinessQuery.error} onRetry={() => readinessQuery.refetch()} />;
  }
  if (!readiness) {
    return (
      <Card aria-live="polite" padding={24}>
        <p className="font-medium">Checking filler health…</p>
        <p className="text-muted-foreground mt-1 text-sm">Finding out what's ready and what's on the way.</p>
      </Card>
    );
  }

  const action = readinessAction(readiness);
  const shared = sharedPool(readiness.pool.channels);

  return (
    <Stack gap="section">
      {/* Normal is quiet and the exception is loud (#1817, decided on #1908). Amber is the brand's
          action colour, so a state where nothing needs doing doesn't wear it: the ready card is
          plain, with the same green dot as the healthy watch line above it. */}
      {action ? (
        <Card className="border-caution/40! bg-caution/5!" padding={20}>
          <div className="flex flex-col items-start gap-4 sm:flex-row">
            <div className="min-w-0 flex-1">
              <StatusLabel tone="warn">Action recommended</StatusLabel>
              <h2 className="mt-3 text-xl font-semibold">{action.title}</h2>
              <p className="text-muted-foreground mt-1 max-w-3xl text-sm">{action.description}</p>
              {/* Every destination behind the button is admin-only, so a member got a button
                  that bounced. They get the state and who can act on it instead. */}
              {isAdmin ? null : (
                <p className="text-muted-foreground mt-2 text-sm">
                  An admin can sort this out from Filler › Manage.
                </p>
              )}
            </div>
            {isAdmin ? (
              <Action
                accessibilityRole="link"
                render={
                  action.section ? (
                    <Link to="/filler/settings/$section" params={{ section: action.section }} />
                  ) : (
                    <Link to={action.to} />
                  )
                }
              >
                {action.label}
              </Action>
            ) : null}
          </div>
        </Card>
      ) : (
        <Card alignItems="center" flexDirection="row" gap={10} padding={16}>
          <span aria-hidden className="bg-lock size-2 shrink-0 rounded-full" />
          <div className="min-w-0">
            <h2 className="font-semibold">Filler is working on its own</h2>
            <p className="text-muted-foreground text-sm">
              Loomarr is preparing clips automatically. Nothing needs your attention.
            </p>
          </div>
        </Card>
      )}

      <section aria-labelledby="coverage-heading">
        <Stack align="end" className="mb-3" direction="row" gap="control" justify="between" wrap>
          <div>
            <h2 id="coverage-heading" className="text-lg font-semibold">
              Channel coverage
            </h2>
            <p className="text-muted-foreground text-sm">
              See how much filler each channel has ready to play.
            </p>
          </div>
          <Action accessibilityRole="link" render={<Link to="/filler/library" />} variant="outline">
            Browse library
          </Action>
        </Stack>
        {readiness?.pool.channels.length ? (
          <div className="grid grid-cols-[repeat(auto-fill,minmax(320px,1fr))] gap-3">
            {shared ? (
              // Every channel draws the same pool, so one card says it once instead of repeating
              // identical numbers per channel (#2733).
              <CoverageCard
                coverage={shared}
                title={
                  <Text as="p" textRole="label">
                    All {readiness.pool.channels.length} channels
                  </Text>
                }
              />
            ) : (
              readiness.pool.channels.map((channel) => (
                <CoverageCard
                  key={channel.channelId}
                  coverage={channel}
                  title={
                    <Link
                      className="font-medium hover:underline"
                      to="/channels/$id/filler"
                      params={{ id: channel.channelId }}
                      search={{ from: "filler" }}
                    >
                      {channel.number} · {channel.name}
                    </Link>
                  }
                />
              ))
            )}
          </div>
        ) : (
          <Card className="text-muted-foreground text-sm" padding={16}>
            {readinessQuery.error
              ? "Channel coverage is temporarily unavailable."
              : "No live channels need filler coverage yet."}
          </Card>
        )}
      </section>
    </Stack>
  );
};

export { FillerOverview, readinessAction };
