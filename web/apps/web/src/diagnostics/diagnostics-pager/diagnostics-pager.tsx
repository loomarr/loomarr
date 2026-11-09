import { Action, Tooltip } from "@loomarr/design-system";

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
      <Action disabled={!canGoNewer} icon="pageNewer" onPress={onNewer} variant="ghost">
        Newer
      </Action>
    </Tooltip>
    <span className="flex flex-col items-center text-xs">
      <strong className="text-foreground font-medium">Page {page}</strong>
      <span className="text-muted-foreground">Up to 50 {itemLabel}</span>
    </span>
    <Tooltip content="Go further back in time">
      <Action disabled={!canGoOlder} icon="pageOlder" iconPlacement="end" onPress={onOlder} variant="ghost">
        Older
      </Action>
    </Tooltip>
  </nav>
);

export { DiagnosticsPager };
