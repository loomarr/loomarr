// PROTOTYPE — throwaway (#2151 step 2). Variant C shared bits: artwork stand-in, state chips,
// breadcrumb and key/value rows. Design-system primitives only; Tailwind is layout glue.
import { Action, ArtworkFrame, Badge, StatusDot, Text } from "@loomarr/design-system";
import type { ReactNode } from "react";
import type { FactsState, Provider } from "./prototype-data";

// A hue-lit stand-in for artwork: never a remote image.
const Art = ({ hue, title, width = 40 }: { hue: number; title: string; width?: number }) => (
  <ArtworkFrame
    alignItems="center"
    height={Math.round(width * 1.5)}
    justifyContent="center"
    state="ready"
    style={{ backgroundColor: `hsl(${hue} 38% 24%)`, flexShrink: 0 }}
    width={width}
  >
    <Text style={{ color: `hsl(${hue} 60% 78%)` }} textRole={width > 60 ? "title" : "metadata"}>
      {title.replace(/^The /, "").slice(0, 1)}
    </Text>
  </ArtworkFrame>
);

const FACTS: Record<FactsState, { label: string; tone: "ok" | "pending" | "progress" | "error" }> = {
  waiting: { label: "waiting", tone: "pending" },
  reading: { label: "reading", tone: "progress" },
  read: { label: "✓ read", tone: "ok" },
  measured: { label: "✓ measured", tone: "ok" },
  failed: { label: "! failed", tone: "error" },
};

const FactsChip = ({ facts, detail }: { facts: FactsState; detail?: string }) => {
  const f = FACTS[facts];
  return (
    <span className="inline-flex items-center gap-2">
      <StatusDot label="" size="compact" tone={f.tone} />
      <Text textRole="metadata" tone={facts === "failed" ? "danger" : "secondary"}>
        {detail ?? f.label}
      </Text>
    </span>
  );
};

const SourceChip = ({ source }: { source: Provider | string }) => (
  <Badge density="pointer" shape="square" tone="neutral">
    {source}
  </Badge>
);

const Crumbs = ({ trail, onBack }: { trail: string[]; onBack: () => void }) => (
  <nav aria-label="Breadcrumb" className="flex flex-wrap items-center gap-1">
    <Action icon="back" onPress={onBack} variant="link">
      {trail[0] ?? ""}
    </Action>
    {trail.slice(1).map((t) => (
      <span className="inline-flex items-center gap-1" key={t}>
        <Text textRole="metadata" tone="muted">
          ›
        </Text>
        <Text textRole="compact" tone="secondary">
          {t}
        </Text>
      </span>
    ))}
  </nav>
);

const Kv = ({ label, children }: { label: string; children: ReactNode }) => (
  <div className="flex flex-col gap-1 sm:flex-row sm:gap-4">
    <div className="shrink-0 sm:w-36">
      <Text shout textRole="metadata" tone="muted">
        {label}
      </Text>
    </div>
    <div className="min-w-0 flex-1">{children}</div>
  </div>
);

const SectionLabel = ({ children }: { children: ReactNode }) => (
  <Text shout textRole="metadata" tone="muted">
    {children}
  </Text>
);

const Warn = ({ children }: { children: ReactNode }) => (
  <span className="inline-flex items-center gap-1">
    <StatusDot label="" size="compact" tone="warn" />
    <Text textRole="metadata" tone="warning">
      {children}
    </Text>
  </span>
);

export { Art, Crumbs, FactsChip, Kv, SectionLabel, SourceChip, Warn };
