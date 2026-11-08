// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// Variant A — Today. The surfaces the Guide page ships, fed stub data: GuideGrid with the
// programme preview on a desktop, GuideCompact (the phone guide) on a phone. The page header is a
// stand-in; everything below it is the real component.
import type { GuideSelection } from "@loomarr/core/guide";
import { GuideCompact, type GuideFilter, GuideGrid, GuideProgrammeDetail } from "@loomarr/ui";
import { useState } from "react";
import { Segmented, type VariantProps, ZOOM_OPTIONS } from "./controls";
import { NOW_MS } from "./data";

const GuideA = ({ device, layout, onZoom, zoom }: VariantProps) => {
  const first = layout.channels[0]?.airings.find((a) => a.isOnNow);
  const [selection, setSelection] = useState<GuideSelection | undefined>(
    first
      ? { anchorMs: NOW_MS, channelId: first.channelId, scheduleBlockId: first.scheduleBlockId }
      : undefined,
  );
  const [filter, setFilter] = useState<GuideFilter>("all");
  const phone = device === "phone";
  return (
    <div className={`flex h-full flex-col gap-3 ${phone ? "pt-4" : "px-6 pt-5 pb-4"}`}>
      <header className={`flex items-end justify-between gap-3 ${phone ? "px-4" : ""}`}>
        <h1 className="text-static-100 text-xl font-semibold">Guide</h1>
        <Segmented label="Time shown" options={ZOOM_OPTIONS} value={zoom} onChange={onZoom} />
      </header>
      <div className="relative flex min-h-0 flex-1">
        {phone ? (
          <GuideCompact
            layout={layout}
            nowMs={NOW_MS}
            filter={filter}
            onFilterChange={setFilter}
            selection={selection}
            onSelect={setSelection}
            onWatch={() => undefined}
          />
        ) : (
          <GuideGrid
            layout={layout}
            nowMs={NOW_MS}
            onSelect={setSelection}
            selection={selection}
            renderPreview={(sel) => <GuideProgrammeDetail focused={false} layout={layout} selection={sel} />}
          />
        )}
      </div>
    </div>
  );
};

export { GuideA };
