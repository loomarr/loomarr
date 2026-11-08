import { Tooltip } from "@loomarr/design-system";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";

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
    className="border-border bg-card flex items-center justify-between rounded-lg border px-2 py-1.5"
  >
    <Tooltip content="Go toward the newest matching results">
      <Button variant="ghost" size="sm" disabled={!canGoNewer} onClick={onNewer}>
        <ChevronLeft aria-hidden /> Newer
      </Button>
    </Tooltip>
    <span className="flex flex-col items-center text-xs">
      <strong className="text-foreground font-medium">Page {page}</strong>
      <span className="text-muted-foreground">Up to 50 {itemLabel}</span>
    </span>
    <Tooltip content="Go further back in time">
      <Button variant="ghost" size="sm" disabled={!canGoOlder} onClick={onOlder}>
        Older <ChevronRight aria-hidden />
      </Button>
    </Tooltip>
  </nav>
);

export { DiagnosticsPager };
