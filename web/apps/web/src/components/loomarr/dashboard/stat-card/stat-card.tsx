import { Card } from "@/components/ui";
import { cn } from "@/lib";
import type { StatCardProps } from "./stat-card.type";

// StatCard — one number on the dashboard (§12, V16), over a label and a one-line explanation.
//
// The mock makes each card a LINK to the surface it summarizes, which is the whole point: a
// dashboard that only reports is a wall of numbers, while one that routes is a way in. The
// caller supplies the link by wrapping; this stays presentational so the gallery renders it
// without a router.
const TONE: Record<NonNullable<StatCardProps["tone"]>, string> = {
  neutral: "text-muted-foreground",
  onair: "text-onair-300",
  suggest: "text-suggest-300",
  tune: "text-tune",
  signal: "text-signal",
};

const StatCard = ({ label, value, note, tone = "neutral", className }: StatCardProps) => (
  <Card className={cn("flex flex-col gap-1.5 p-4", className)}>
    <p className="font-mono text-2xs text-muted-foreground uppercase tracking-wide">{label}</p>
    <p className={cn("font-mono font-semibold text-2xl leading-none", TONE[tone])}>{value}</p>
    <p className="text-muted-foreground text-xs">{note}</p>
  </Card>
);

export { StatCard };
