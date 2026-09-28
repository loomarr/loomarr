import { cn } from "@/lib/utils";
import { Button } from "../button";
import { StatusDot } from "../status-dot";
import type { StatusStripActionProps, StatusStripProps, StatusStripTone } from "./status-strip.type";

// StatusStrip — Home's one-line headline: how things are, then what needs someone (#1659 web
// mock). "All 6 channels are playing | 3 requests need you [Review] | Restart Loomarr to finish
// saving 2 settings [Restart…]".
//
// A `role="status"` region, so a change of headline (playing → something needs fixing) is
// announced once, politely, without moving focus. The items inside are plain content; the region
// is the only live part.
const HERO: Record<StatusStripTone, string> = {
  ok: "bg-lock ring-4 ring-lock/20",
  error: "bg-onair ring-4 ring-onair/20",
  idle: "bg-static-500",
};

const StatusStrip = ({ tone, loading = false, title, items = [], action, className }: StatusStripProps) => (
  <div
    role="status"
    className={cn(
      "flex flex-wrap items-center gap-x-3.5 gap-y-2.5 rounded-lg border bg-static-900 px-4 py-3",
      tone === "error" ? "border-onair/40" : "border-static-700",
      className,
    )}
  >
    <span
      aria-hidden="true"
      className={cn(
        "size-2.5 shrink-0 rounded-full",
        HERO[tone],
        loading && "motion-safe:animate-[onair-pulse_1.2s_ease-in-out_infinite]",
      )}
    />
    <span className="font-semibold text-[15px]">{title}</span>
    {items.length > 0 && <span aria-hidden="true" className="h-[18px] w-px bg-static-700" />}
    {items.map((item) => (
      <span key={item.id} className="flex items-center gap-2 text-[13px]">
        <StatusDot tone={item.tone} label="" className="size-1.5" />
        {item.title}
        {item.action}
      </span>
    ))}
    {action != null && <div className="ml-auto">{action}</div>}
  </div>
);

// StatusStripAction — an item's 28 px button ("Review", "Restart…", "Edit and retry").
const StatusStripAction = ({ primary = false, className, ...props }: StatusStripActionProps) => (
  <Button
    size="sm"
    variant={primary ? "default" : "outline"}
    className={cn("h-7 px-2.5", className)}
    {...props}
  />
);

export { StatusStrip, StatusStripAction };
