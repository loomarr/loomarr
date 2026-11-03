// PROTOTYPE — throwaway (#2151 step 2). The setup wizard's Library step (decision 30), full-screen
// without the app shell. Frame copied from WizardShell (rail of steps + one step card + Back/Next)
// but drawn with design-system primitives. The step never blocks on the import itself.
import { Action, BrandMark, Icon, icons, Surface, Text } from "@loomarr/design-system";
import { useState } from "react";
import { wizardSteps } from "@/wizard/steps";
import { type Root, scanStatus } from "./prototype-data";
import type { Device, ScanState } from "./prototype-state";
import { ScanHealth } from "./scan-health";
import { FolderCard, Line, MOUNT_HINT, Stack } from "./scan-health-parts";

const STEPS = wizardSteps("tunarr");
const CURRENT = "library";

// Narrow screens: the rail collapses to one line, "Step 5 of 7 · Library", over the brand bar.
const CompactRail = () => {
  const at = STEPS.findIndex((s) => s.id === CURRENT);
  return (
    <Stack gap="inline" paddingHorizontal="section" paddingTop="section">
      <BrandMark contained={false} decorative size={4} width={56} />
      <Text textRole="label" tone="secondary">
        {`Step ${at + 1} of ${STEPS.length} · ${STEPS[at]?.title ?? ""}`}
      </Text>
    </Stack>
  );
};

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
          Saved as you go
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

// The brand chroma bar (as in the logo mark): short, left-aligned, every segment lit.
const ChromaBars = () => (
  <Stack alignItems="flex-start">
    <BrandMark contained={false} decorative size={6} width={84} />
  </Stack>
);

const RootChoice = ({
  root,
  confirmed,
  onConfirm,
}: {
  root: Root;
  confirmed: boolean;
  onConfirm: () => void;
}) => (
  <FolderCard
    root={root}
    using={confirmed}
    checked={confirmed && root.walked !== "not yet" ? root.walked : "Not checked yet"}
    actions={
      confirmed ? null : (
        <Action onPress={onConfirm} variant="secondary" disabled={!root.readable}>
          Use this folder
        </Action>
      )
    }
  />
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
        <Stack gap="control">
          <div className="hidden md:block">
            <ChromaBars />
          </div>
          <Text as="h1" textRole="title">
            Where's your media?
          </Text>
          <Text textRole="body" tone="secondary">
            So Loomarr can put your shows and movies on the air. We found these through Emby, Sonarr and
            Radarr — pick the ones that hold your library.
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
              Check again
            </Action>
            {rechecks > 0 ? (
              <Text textRole="metadata" tone="secondary">
                Checked just now
              </Text>
            ) : null}
          </Line>

          {importing ? (
            <>
              <ScanHealth status={status} density="full" inFlow />
              <Text textRole="body" tone="secondary">
                You can carry on — titles appear as they're ready.
              </Text>
            </>
          ) : null}
        </Surface>

        {hasReadableConfirmed ? null : (
          <Text textRole="body" tone="secondary">
            {MOUNT_HINT}
          </Text>
        )}
        <Line gap="control">
          <Action variant="ghost">Back</Action>
          <Stack marginLeft="auto">
            <Action
              {...(hasReadableConfirmed ? { tone: "primary" as const } : { variant: "secondary" as const })}
              disabled={!hasReadableConfirmed}
            >
              Continue
            </Action>
          </Stack>
        </Line>
      </Stack>
    </Stack>
  );
};

const WizardLibraryStep = ({ scan }: { scan: ScanState; device: Device }) => (
  <Stack
    flexDirection="row"
    flexWrap="wrap"
    alignContent="flex-start"
    minHeight="100vh"
    backgroundColor="surfaceCanvas"
  >
    <div className="hidden md:flex">
      <Rail />
    </div>
    <div className="w-full md:hidden">
      <CompactRail />
    </div>
    <Body key={scan} scan={scan} />
  </Stack>
);

export { WizardLibraryStep };
