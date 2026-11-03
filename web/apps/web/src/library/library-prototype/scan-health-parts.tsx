// PROTOTYPE — throwaway (#2151 step 2). Small pieces shared by ScanHealth, Settings → Library and
// the wizard step: a layout stack, the warning/bad callout, the stage stepper, a scan-root row.
import { Action, Badge, ProgressTrack, StatusDot, Surface, Text } from "@loomarr/design-system";
import type { ComponentProps, ReactNode } from "react";
import type { Root, ScanStatus, Stage } from "./prototype-data";

// Transparent layout box: Surface without its card look. Column by default, like the web host.
const Stack = (props: ComponentProps<typeof Surface>) => (
  <Surface backgroundColor="transparent" borderWidth={0} {...props} />
);
const Line = ({ children, ...props }: ComponentProps<typeof Surface>) => (
  <Stack flexDirection="row" alignItems="center" flexWrap="wrap" gap="inline" {...props}>
    {children}
  </Stack>
);

const toneDot = { ok: "ok", busy: "progress", warn: "warn", bad: "error" } as const;
const dotFor = (tone: ScanStatus["tone"]) => toneDot[tone];

const fmt = (n: number) => n.toLocaleString("en-US");
const pct = (p: NonNullable<ScanStatus["progress"]>) => Math.round((p.done / p.total) * 100);

// Amber = attention (held removal, unconfirmed roots); red = something is down (offline root).
const Callout = ({
  tone,
  title,
  children,
  actions,
}: {
  tone: "warn" | "bad";
  title: string;
  children?: ReactNode;
  actions?: ReactNode;
}) => (
  <Surface
    role="alert"
    backgroundColor={tone === "warn" ? "stateWarningSurface" : "stateDangerSurface"}
    borderColor={tone === "warn" ? "stateWarning" : "stateDanger"}
    borderRadius="control"
    padding="control"
    gap="inline"
  >
    <Text textRole="label" tone={tone === "warn" ? "warning" : "danger"}>
      {title}
    </Text>
    {children ? (
      <Text textRole="body" tone="secondary">
        {children}
      </Text>
    ) : null}
    {actions ? <Line gap="control">{actions}</Line> : null}
  </Surface>
);

const stageDot = { done: "ok", running: "progress", waiting: "off" } as const;
const stageWord = { done: "done", running: "running", waiting: "waiting" } as const;

const StageCell = ({ stage, status }: { stage: Stage; status: ScanStatus }) => {
  const running = stage.state === "running" && status.progress;
  return (
    <Stack gap="inline">
      <Line>
        <StatusDot
          tone={status.paused && stage.state === "running" ? "pending" : stageDot[stage.state]}
          label={`${stage.label}: ${stageWord[stage.state]}`}
        />
        <Text textRole="label" tone={stage.state === "waiting" ? "muted" : "primary"}>
          {stage.label}
        </Text>
      </Line>
      {running ? (
        <>
          <ProgressTrack
            width="100%"
            percent={pct(running)}
            label={`${stage.label} ${pct(running)} percent`}
          />
          <Text textRole="metadata" tone="secondary">
            {fmt(running.done)} / {fmt(running.total)} · {running.eta}
            {status.paused ? "" : ` · ${running.rate}`}
          </Text>
        </>
      ) : (
        <Text textRole="metadata" tone="muted">
          {stageWord[stage.state]}
        </Text>
      )}
    </Stack>
  );
};

const StageStepper = ({ status }: { status: ScanStatus }) => (
  <Stack flexDirection="row" flexWrap="wrap" gap="control" role="list" aria-label="Scan stages">
    {status.stages.map((stage) => (
      <Stack key={stage.key} role="listitem" flexBasis={150} flexGrow={1}>
        <StageCell stage={stage} status={status} />
      </Stack>
    ))}
  </Stack>
);

// One scan root: path, where discovery saw it, readable, videos, last walk.
const RootFacts = ({ root }: { root: Root }) => (
  <Line gap="control">
    <Badge tone={root.readable ? "success" : "danger"}>
      {root.readable ? "readable ✓" : "not readable ✗"}
    </Badge>
    <Text textRole="metadata" tone="secondary">
      found in {root.foundIn.join(", ")}
    </Text>
    <Text textRole="metadata" tone="secondary">
      {fmt(root.videos)} videos
    </Text>
    <Text textRole="metadata" tone="secondary">
      walked {root.walked}
    </Text>
  </Line>
);

const MOUNT_HINT = "Loomarr reads your media files directly — mount them read-only, then Re-check.";

export type { Root };
export { Action, Callout, dotFor, fmt, Line, MOUNT_HINT, pct, RootFacts, Stack, StageStepper };
