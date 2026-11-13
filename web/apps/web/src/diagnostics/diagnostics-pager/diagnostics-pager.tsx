import { Action, Stack, Text, Tooltip } from "@loomarr/design-system";

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
    <Stack align="center" flexShrink={1} render="span">
      <Text as="strong" textRole="label" tone="primary">
        Page {page}
      </Text>
      <Text as="span" textRole="hint" tone="secondary">
        Up to 50 {itemLabel}
      </Text>
    </Stack>
    <Tooltip content="Go further back in time">
      <Action disabled={!canGoOlder} icon="pageOlder" iconPlacement="end" onPress={onOlder} variant="ghost">
        Older
      </Action>
    </Tooltip>
  </Stack>
);

export { DiagnosticsPager };
