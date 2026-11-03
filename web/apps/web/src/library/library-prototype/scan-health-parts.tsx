// PROTOTYPE — throwaway (#2151 step 2). Small pieces shared by ScanHealth, Settings → Library and
// the wizard step: layout stacks, the calm callout, the lit progress bar, the "ready" poster chips
// and the media-folder card. Plain words only (see PLAIN-WORDS.md).
import {
  Action,
  Icon,
  icons,
  StatusDot,
  Surface,
  Text,
  useReducedMotionPreference,
} from "@loomarr/design-system";
import { type ComponentProps, type ReactNode, useEffect, useState } from "react";
import { itemsFor, type Root, type ScanStatus } from "./prototype-data";

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

// True only when the system positively allows motion (null while still unknown counts as "no").
const useMotionAllowed = () => useReducedMotionPreference() === false;

// Amber = attention (missing movies, folders not chosen yet); firm but never alarming.
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
    borderColor={tone === "warn" ? "stateWarning" : "stateDanger"}
    borderRadius="control"
    padding="section"
    gap="control"
  >
    <Text textRole="headline" tone={tone === "warn" ? "warning" : "danger"}>
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

// The calm, alive progress bar: amber fill with a soft lit edge. The glow breathes only when
// motion is allowed; otherwise it holds still.
const LitBar = ({ percent, label, paused }: { percent: number; label: string; paused?: boolean }) => {
  const motion = useMotionAllowed();
  return (
    <Surface
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={percent}
      backgroundColor="surfaceCanvas"
      borderWidth={0}
      borderRadius="round"
      height={8}
      width="100%"
      overflow="visible"
    >
      <Surface
        backgroundColor="actionPrimary"
        borderWidth={0}
        borderRadius="round"
        height={8}
        width={`${percent}%`}
        opacity={paused ? 0.5 : 1}
        style={{
          boxShadow: paused ? "none" : "0 0 10px 1px rgba(255, 184, 77, 0.55)",
          transition: motion ? "width 600ms ease-out" : "none",
        }}
      />
    </Surface>
  );
};

// A small hue-lit poster tile (never a remote image) that slides in, one after another.
const ReadyChip = ({ title, hue, index }: { title: string; hue: number; index: number }) => {
  const motion = useMotionAllowed();
  const [shown, setShown] = useState(false);
  useEffect(() => {
    const t = setTimeout(() => setShown(true), 200 + index * 220);
    return () => clearTimeout(t);
  }, [index]);
  const visible = shown || !motion;
  return (
    <div
      style={{
        opacity: visible ? 1 : 0,
        transform: visible ? "none" : "translateY(8px)",
        transition: motion ? "opacity 300ms ease-out, transform 300ms ease-out" : "none",
      }}
    >
      <Surface
        width={64}
        height={92}
        borderRadius="control"
        borderWidth={0}
        alignItems="center"
        justifyContent="center"
        padding="inline"
        style={{ backgroundColor: `hsl(${hue} 38% 24%)` }}
        aria-label={`${title} is ready`}
        role="img"
      >
        <Text textRole="label" style={{ color: `hsl(${hue} 60% 78%)`, textAlign: "center" }}>
          {title}
        </Text>
      </Surface>
    </div>
  );
};

// The latest few titles that became ready, while the import runs.
const ReadyChips = () => {
  const ready = itemsFor("idle").slice(0, 5);
  return (
    <Stack gap="inline">
      <Text textRole="label" tone="secondary">
        Ready so far
      </Text>
      <Line gap="control">
        {ready.map((it, i) => (
          <ReadyChip key={it.id} title={it.title} hue={it.hue} index={i} />
        ))}
      </Line>
    </Stack>
  );
};

// Static → clear in 200ms (the onboarding "lock in" idiom). Still when motion is reduced.
const LockIn = ({ locked, children }: { locked: boolean; children: ReactNode }) => {
  const motion = useMotionAllowed();
  return (
    <div
      style={{
        filter: locked ? "none" : "grayscale(1)",
        transition: motion ? "filter 200ms ease-out" : "none",
      }}
    >
      {children}
    </div>
  );
};

// "/media/tv" → "TV", "/media/movies" → "Movies", anything else → its last folder name.
const folderName = (path: string) => {
  const last = path.split("/").filter(Boolean).pop() ?? path;
  return last.length <= 2 ? last.toUpperCase() : last.charAt(0).toUpperCase() + last.slice(1);
};

const joinNames = (names: string[]) =>
  names.length <= 1 ? (names[0] ?? "") : `${names.slice(0, -1).join(", ")} and ${names[names.length - 1]}`;

const foundThrough = (foundIn: string[]) =>
  foundIn.includes("added by hand") ? "Added by you" : `Found through ${joinNames(foundIn)}`;

// One media folder as a card: big human name, small muted path, then what Loomarr knows.
const FolderCard = ({
  root,
  using,
  checked,
  actions,
}: {
  root: Root;
  using: boolean;
  checked: string; // "12 minutes ago" / "Not checked yet"
  actions?: ReactNode;
}) => (
  <Surface
    padding="section"
    gap="control"
    borderRadius="control"
    borderColor={root.readable ? "borderDecorative" : "stateWarning"}
  >
    <Line gap="control" alignItems="flex-start" flexWrap="nowrap">
      <Stack flexGrow={1} flexShrink={1} flexBasis={0} gap="inline">
        <Text as="h3" textRole="headline">
          {folderName(root.path)}
        </Text>
        <Text textRole="data" tone="muted">
          {root.path}
        </Text>
      </Stack>
      {using && root.readable ? (
        <Text textRole="label" tone="success">
          In use
        </Text>
      ) : null}
    </Line>
    <LockIn locked={root.readable ? using : true}>
      <Line gap="inline" flexWrap="nowrap">
        {root.readable ? (
          <>
            <Icon decorative glyph={icons.success} size="compact" tone="success" />
            <Text textRole="body" tone="success">
              Loomarr can open this folder
            </Text>
          </>
        ) : (
          <>
            <StatusDot tone="warn" label="Can't open" />
            <Text textRole="body" tone="warning">
              Loomarr can't open this folder right now
            </Text>
          </>
        )}
      </Line>
    </LockIn>
    <Text textRole="body" tone="secondary">
      {foundThrough(root.foundIn)}
      {" · "}
      {checked === "Not checked yet" ? checked : `Checked ${checked}`}
    </Text>
    {root.readable ? null : (
      <Text textRole="body" tone="secondary">
        {MOUNT_HINT}
      </Text>
    )}
    {actions ? <Line gap="control">{actions}</Line> : null}
  </Surface>
);

const MOUNT_HINT =
  "Loomarr needs to open your media folders itself. Make them available to Loomarr (read-only is fine), then Check again.";

export type { Root };
export {
  Action,
  Callout,
  dotFor,
  FolderCard,
  fmt,
  Line,
  LitBar,
  MOUNT_HINT,
  pct,
  ReadyChips,
  Stack,
  useMotionAllowed,
};
