import { Action, Chip, CompactSelect, Stack, Text } from "@loomarr/design-system";
import { useMemo } from "react";
import {
  dayLabel,
  FORWARD_DAYS,
  RETENTION_DAYS,
  START_HOUR_OPTIONS,
  WINDOW_CHOICES,
} from "../guide-page-model";

interface GuideToolbarProps {
  anchorMs: number;
  /** The day, hour and start are all at their defaults: the window follows now. */
  atNow: boolean;
  dayOffset: number;
  /** A precise start is pinned, so the View options trigger announces it. */
  hasCustomView: boolean;
  setDayOffset: (offset: number) => void;
  setHourShift: (update: (hours: number) => number) => void;
  setStartHour: (hour: number | null) => void;
  setViewOptionsOpen: (update: (open: boolean) => boolean) => void;
  setWindowMinutes: (minutes: number) => void;
  startHour: number | null;
  timeZone?: string;
  viewOptionsOpen: boolean;
  windowMinutes: number;
}

// The full grid's controls: the day, the hour stepper and the span stay visible; the precise start
// lives in a secondary row behind View.
const GuideToolbar = ({
  anchorMs,
  atNow,
  dayOffset,
  hasCustomView,
  setDayOffset,
  setHourShift,
  setStartHour,
  setViewOptionsOpen,
  setWindowMinutes,
  startHour,
  timeZone,
  viewOptionsOpen,
  windowMinutes,
}: GuideToolbarProps) => {
  const dayChoices = useMemo(() => {
    const out: number[] = [];
    for (let d = -RETENTION_DAYS; d <= FORWARD_DAYS; d++) out.push(d);
    return out;
  }, []);

  return (
    <>
      <div className="border-border/60 flex flex-wrap items-center gap-3.5 border-b px-7 py-2.5">
        <Stack align="center" direction="row" flexShrink={1} gap="1.75">
          <div>
            <CompactSelect
              label="Day"
              onValueChange={(v) => setDayOffset(Number(v))}
              options={dayChoices.map((d) => ({
                label: dayLabel(d, anchorMs, timeZone),
                value: String(d),
              }))}
              value={String(dayOffset)}
            />
          </div>

          {/* A past day is RECOMPUTED from the current lineup, not a recording — and a
              future one is a plan. The chip says which, and doubles as the way back. */}
          {dayOffset !== 0 && (
            <Chip mono onPress={() => setDayOffset(0)} tone={dayOffset < 0 ? "muted" : "info"}>
              {`${dayOffset < 0 ? "AS AIRED" : "SCHEDULED"} · back to today`}
            </Chip>
          )}
        </Stack>

        {/* Hour stepper. NOW is the anchor an operator returns to; the arrows nudge the
            window without changing the day. */}
        <Stack align="center" direction="row" flexShrink={1} gap="1">
          <Action icon="earlier" layout="icon" onPress={() => setHourShift((h) => h - 1)} variant="outline">
            Back an hour
          </Action>
          {/* Already at now, the anchor is the quiet choice; away from it, the way back. */}
          <Action
            onPress={() => {
              setHourShift(() => 0);
              setDayOffset(0);
              setStartHour(null);
            }}
            variant={atNow ? "ghost" : "outline"}
          >
            NOW
          </Action>
          <Action icon="later" layout="icon" onPress={() => setHourShift((h) => h + 1)} variant="outline">
            Forward an hour
          </Action>
        </Stack>

        {/* Span stays in the primary row: it changes the question the guide answers, and
            four compact choices are faster to scan than a select. */}
        <Stack align="center" className="sm:ml-auto" direction="row" flexShrink={1} gap="1.75">
          <Text as="span" textRole="metadata">
            Span
          </Text>
          <Stack direction="row" gap="1">
            {WINDOW_CHOICES.map((w) => {
              const active = windowMinutes === Number(w.value);
              return (
                <Chip
                  accessibilityLabel={`Show ${w.description}`}
                  key={w.value}
                  mono
                  onPress={() => setWindowMinutes(Number(w.value))}
                  selected={active}
                >
                  {w.label}
                </Chip>
              );
            })}
          </Stack>
        </Stack>

        <Action
          accessibilityLabel={hasCustomView ? "View options, custom" : "View options"}
          aria-controls="guide-view-options"
          aria-expanded={viewOptionsOpen}
          icon="filter"
          onPress={() => setViewOptionsOpen((open) => !open)}
          variant="outline"
        >
          {hasCustomView ? "View · custom" : "View"}
        </Action>
      </div>

      {viewOptionsOpen && (
        <Stack
          align="center"
          className="border-border/60 bg-card/40 border-b"
          columnGap="6"
          direction="row"
          id="guide-view-options"
          justify={{ base: "start", sm: "end" }}
          paddingHorizontal={28}
          paddingVertical={10}
          rowGap="2"
          wrap
        >
          {/* Pin the window to a specific part of the chosen day without stepping there
              one hour at a time. */}
          <Stack align="center" direction="row" gap="1.75">
            <Text as="span" textRole="metadata">
              Start at
            </Text>
            <div>
              <CompactSelect
                label="Start hour"
                onValueChange={(v) => setStartHour(v === "auto" ? null : Number(v))}
                options={START_HOUR_OPTIONS}
                value={startHour === null ? "auto" : String(startHour)}
              />
            </div>
          </Stack>
        </Stack>
      )}
    </>
  );
};

export { GuideToolbar };
