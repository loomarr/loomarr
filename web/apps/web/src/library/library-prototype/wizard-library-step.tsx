// PROTOTYPE — throwaway (#2151 step 2). The setup wizard's Library step (decision 30), full-screen
// without the app shell. Frame copied from WizardShell (rail of steps + one step card + Back/Next)
// but drawn with design-system primitives. The step never blocks on the import itself.
import { Action, Badge, Icon, icons, Surface, Text } from "@loomarr/design-system";
import { useState } from "react";
import { wizardSteps } from "@/wizard/steps";
import { type Root, scanStatus } from "./prototype-data";
import type { Device, ScanState } from "./prototype-state";
import { ScanHealth } from "./scan-health";
import { Line, MOUNT_HINT, RootFacts, Stack } from "./scan-health-parts";

const STEPS = wizardSteps("tunarr");
const CURRENT = "library";

const Rail = () => {
  const at = STEPS.findIndex((s) => s.id === CURRENT);
  return (
    <Surface
      borderRadius={0}
      borderWidth={0}
      borderRightWidth={1}
      padding="section"
      gap="section"
      flexBasis={272}
      flexShrink={0}
    >
      <Stack gap="inline">
        <Text textRole="label" tracking={0.16}>
          LOOMARR
        </Text>
        <Text textRole="label">Let's get you on the air</Text>
        <Text textRole="metadata" tone="muted">
          First-run setup · saved as you go
        </Text>
      </Stack>
      <Stack role="list" gap="inline" aria-label="Setup steps">
        {STEPS.map((step, i) => {
          const current = step.id === CURRENT;
          return (
            <Line
              key={step.id}
              gap="control"
              aria-current={current ? "step" : undefined}
              paddingVertical="inline"
            >
              <Surface
                width={24}
                height={24}
                borderRadius="round"
                alignItems="center"
                justifyContent="center"
                borderColor={i < at ? "stateSuccess" : current ? "contentPrimary" : "borderDecorative"}
              >
                {i < at ? (
                  <Icon decorative glyph={icons.success} size="compact" tone="success" />
                ) : (
                  <Text textRole="metadata" tone={current ? "primary" : "muted"}>
                    {i + 1}
                  </Text>
                )}
              </Surface>
              <Text textRole="body" tone={current ? "primary" : "secondary"} strong={current}>
                {step.title}
              </Text>
            </Line>
          );
        })}
      </Stack>
    </Surface>
  );
};

const RootChoice = ({
  root,
  confirmed,
  onConfirm,
}: {
  root: Root;
  confirmed: boolean;
  onConfirm: () => void;
}) => (
  <Surface
    padding="control"
    gap="inline"
    borderRadius="control"
    borderColor={confirmed ? "borderDecorative" : "stateWarning"}
  >
    <Line gap="control">
      <Text textRole="data">{root.path}</Text>
      {confirmed ? <Badge tone="success">confirmed</Badge> : null}
      {confirmed ? null : (
        <Stack marginLeft="auto">
          <Action variant="suggest" onPress={onConfirm}>
            Confirm
          </Action>
        </Stack>
      )}
    </Line>
    <RootFacts root={confirmed ? root : { ...root, walked: "not yet" }} />
    {root.readable ? null : (
      <Text textRole="metadata" tone="danger">
        {MOUNT_HINT}
      </Text>
    )}
  </Surface>
);

const Body = ({ scan }: { scan: ScanState }) => {
  const base = scanStatus(scan);
  const [confirmed, setConfirmed] = useState<ReadonlySet<string>>(
    () => new Set(base.roots.filter((r) => r.confirmed).map((r) => r.path)),
  );
  const [rechecks, setRechecks] = useState(0);
  // Confirming a root on a fresh install starts the import; the step then shows its progress.
  const status = scan === "fresh" && confirmed.size > 0 ? scanStatus("importing") : base;
  const hasReadableConfirmed = base.roots.some((r) => confirmed.has(r.path) && r.readable);
  const importing = confirmed.size > 0;

  return (
    <Stack flex={1} minWidth={0} alignItems="center" padding="section">
      <Stack width="100%" maxWidth={672} gap="section" paddingVertical="section">
        <Stack gap="inline">
          <Text as="h1" textRole="title">
            Where is your media?
          </Text>
          <Text textRole="body" tone="secondary">
            Loomarr found these folders from your media server and Sonarr/Radarr. Confirm the ones that hold
            your library.
          </Text>
        </Stack>

        <Surface padding="section" gap="control">
          {base.roots.map((root) => (
            <RootChoice
              key={root.path}
              root={root}
              confirmed={confirmed.has(root.path)}
              onConfirm={() => setConfirmed((prev) => new Set(prev).add(root.path))}
            />
          ))}
          <Line gap="control">
            <Action variant="outline" onPress={() => setRechecks((n) => n + 1)}>
              Re-check
            </Action>
            {rechecks > 0 ? (
              <Text textRole="metadata" tone="secondary">
                Checked just now
              </Text>
            ) : null}
          </Line>

          {importing ? (
            <>
              <ScanHealth status={status} density="full" />
              <Text textRole="body" tone="secondary">
                You can continue — titles become schedulable as their files are read.
              </Text>
            </>
          ) : null}
        </Surface>

        <Line gap="control">
          <Action variant="ghost">Back</Action>
          <Stack
            marginLeft="auto"
            flexDirection="row"
            alignItems="center"
            gap="control"
            flexWrap="wrap"
            justifyContent="flex-end"
          >
            {hasReadableConfirmed ? null : (
              <Text textRole="metadata" tone="warning">
                {MOUNT_HINT}
              </Text>
            )}
            <Action variant="secondary" disabled={!hasReadableConfirmed}>
              Continue
            </Action>
          </Stack>
        </Line>
      </Stack>
    </Stack>
  );
};

const WizardLibraryStep = ({ scan }: { scan: ScanState; device: Device }) => (
  <Stack flexDirection="row" flexWrap="wrap" minHeight="100vh" backgroundColor="surfaceCanvas">
    <Rail />
    <Body key={scan} scan={scan} />
  </Stack>
);

export { WizardLibraryStep };
