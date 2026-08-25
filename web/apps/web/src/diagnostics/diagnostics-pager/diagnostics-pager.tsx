import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

const DiagnosticsPager = ({
  label,
  page,
  itemLabel,
  canGoNewer,
  canGoOlder,
  onNewer,
  onOlder,
}: {
  label: string;
  page: number;
  itemLabel: string;
  canGoNewer: boolean;
  canGoOlder: boolean;
  onNewer: () => void;
  onOlder: () => void;
}) => (
  <nav
    aria-label={label}
    className="flex items-center justify-between rounded-lg border border-border bg-card px-2 py-1.5"
  >
    <Tooltip>
      <TooltipTrigger
        render={
          <Button variant="ghost" size="sm" disabled={!canGoNewer} onClick={onNewer}>
            <ChevronLeft aria-hidden /> Newer
          </Button>
        }
      />
      <TooltipContent>Go toward the newest matching results</TooltipContent>
    </Tooltip>
    <span className="flex flex-col items-center text-xs">
      <strong className="font-medium text-foreground">Page {page}</strong>
      <span className="text-muted-foreground">Up to 50 {itemLabel}</span>
    </span>
    <Tooltip>
      <TooltipTrigger
        render={
          <Button variant="ghost" size="sm" disabled={!canGoOlder} onClick={onOlder}>
            Older <ChevronRight aria-hidden />
          </Button>
        }
      />
      <TooltipContent>Go further back in time</TooltipContent>
    </Tooltip>
  </nav>
);

export { DiagnosticsPager };
