import { Action, Stack, Tooltip } from "@loomarr/design-system";

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
  <Stack
    align="center"
    aria-label={label}
    className="border-border bg-card rounded-lg border px-2 py-1.5"
    direction="row"
    justify="between"
    render="nav"
  >
    <Tooltip content="Go toward the newest matching results">
      <Action disabled={!canGoNewer} icon="pageNewer" onPress={onNewer} variant="ghost">
        Newer
      </Action>
    </Tooltip>
    <Stack align="center" className="text-xs" render="span">
      <strong className="text-foreground font-medium">Page {page}</strong>
      <span className="text-muted-foreground">Up to 50 {itemLabel}</span>
    </Stack>
    <Tooltip content="Go further back in time">
      <Action disabled={!canGoOlder} icon="pageOlder" iconPlacement="end" onPress={onOlder} variant="ghost">
        Older
      </Action>
    </Tooltip>
  </Stack>
);

export { DiagnosticsPager };
