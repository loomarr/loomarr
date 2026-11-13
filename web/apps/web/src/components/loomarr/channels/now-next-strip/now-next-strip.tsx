import { Text } from "@loomarr/design-system";
import { cn } from "@/lib/utils";
import type { NowNextStripProps } from "./now-next-strip.type";

// NowNextStrip — the "Now: X · Next: Y" line for a channel (§3). Times + titles
// are mono (machine data, §2.2). A flex/pod gap reads as "Filler" rather than dead
// air; an unprogrammed channel reads "Nothing scheduled".
const NowNextStrip = ({ now, next, gap, className }: NowNextStripProps) => {
  if (!now && !gap) {
    return (
      <Text as="p" className={cn(className)} textRole="compact" tone="secondary">
        Nothing scheduled
      </Text>
    );
  }
  return (
    <Text as="p" className={cn("truncate", className)} textRole="compact" tone="secondary">
      Now{" "}
      <Text as="span" textRole="mono" tone="primary">
        {gap ? "Filler break" : now?.title}
      </Text>
      {now?.until && (
        <Text as="span" className="ml-1" textRole="mono" tone="secondary">
          ·til {now.until}
        </Text>
      )}
      {next && (
        <>
          <span className="mx-1.5">·</span>
          Next{" "}
          <Text as="span" textRole="mono" tone="primary">
            {next.title}
          </Text>
        </>
      )}
    </Text>
  );
};

export { NowNextStrip };
