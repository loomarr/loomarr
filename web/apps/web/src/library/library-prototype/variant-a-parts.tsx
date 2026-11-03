// PROTOTYPE — throwaway (#2151 step 2). Variant A: shared pieces (artwork stand-in, chips, side sheet).
import { Action, Badge, type BadgeTone, Surface, Text } from "@loomarr/design-system";
import type { ReactNode } from "react";
import type { FactsState, Provider } from "./prototype-data";

// Hue-lit stand-in for artwork: never a remote image.
const Art = ({ hue, width = 32 }: { hue: number; width?: number }) => (
  <Surface
    aria-hidden
    backgroundColor={`hsl(${hue} 38% 24%)` as never}
    borderRadius={4}
    flexShrink={0}
    height={Math.round(width * 1.5)}
    width={width}
  />
);

const FACTS: Record<FactsState, { label: string; tone: BadgeTone }> = {
  waiting: { label: "waiting", tone: "neutral" },
  reading: { label: "reading", tone: "info" },
  read: { label: "✓ read", tone: "success" },
  measured: { label: "✓ measured", tone: "success" },
  failed: { label: "! failed", tone: "danger" },
};

const FactsChip = ({ state, detail }: { state: FactsState; detail?: string }) => (
  <Surface backgroundColor="transparent" borderWidth={0} gap={2} alignItems="flex-start">
    <Badge tone={FACTS[state].tone}>{FACTS[state].label}</Badge>
    {detail ? <Text textRole="metadata">{detail}</Text> : null}
  </Surface>
);

const SourceChip = ({ source, label }: { source: Provider | string; label?: string }) => (
  <Badge shape="square" tone="neutral">
    {label ?? source}
  </Badge>
);

const SectionLabel = ({ children }: { children: ReactNode }) => (
  <Text shout textRole="metadata">
    {children}
  </Text>
);

// The right-hand sheet over the table. Full-screen on a phone. The table stays visible behind it
// on desktop; clicking the uncovered strip closes it.
interface SideSheetProps {
  title: string;
  subtitle?: string;
  phone: boolean;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
}
const SideSheet = ({ title, subtitle, phone, onClose, children, footer }: SideSheetProps) => (
  <div className="fixed inset-0 z-40 flex justify-end">
    {phone ? null : (
      <button type="button" aria-label="Close sheet" onClick={onClose} className="flex-1 cursor-default" />
    )}
    <Surface
      level="elevated"
      aria-label={title}
      borderRadius={0}
      borderWidth={0}
      borderLeftWidth={phone ? 0 : 1}
      height="100%"
      maxWidth="100%"
      role="dialog"
      width={phone ? "100%" : 540}
    >
      <Surface
        alignItems="center"
        backgroundColor="transparent"
        borderRadius={0}
        borderWidth={0}
        borderBottomWidth={1}
        flexDirection="row"
        gap="control"
        padding="control"
      >
        <Action icon="close" onPress={onClose} variant="ghost" accessibilityLabel="Close">
          Close
        </Action>
        <Surface backgroundColor="transparent" borderWidth={0} flex={1} minWidth={0}>
          <Text numberOfLines={1} textRole="label" tone="primary">
            {title}
          </Text>
          {subtitle ? (
            <Text numberOfLines={1} textRole="metadata">
              {subtitle}
            </Text>
          ) : null}
        </Surface>
      </Surface>
      <div className="min-h-0 flex-1 overflow-y-auto">{children}</div>
      {footer ? (
        <Surface
          backgroundColor="transparent"
          borderRadius={0}
          borderWidth={0}
          borderTopWidth={1}
          flexDirection="row"
          flexWrap="wrap"
          gap="inline"
          padding="control"
        >
          {footer}
        </Surface>
      ) : null}
    </Surface>
  </div>
);

export { Art, FactsChip, SectionLabel, SideSheet, SourceChip };
