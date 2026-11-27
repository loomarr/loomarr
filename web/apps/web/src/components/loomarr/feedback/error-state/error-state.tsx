import { toProblem } from "@loomarr/api/mutator";
import { Action, Card, Icon, icons, Stack, Text } from "@loomarr/design-system";
import type { ErrorStateProps } from "./error-state.type";

// ErrorState — the RFC 7807 renderer (§3). Every failure surfaces the problem's
// `title`/`detail` as words, never raw JSON (§6). Retry is offered only when the
// caller says the operation is idempotent. Field-level 422 errors render inline via
// TanStack Form instead (this is the block/list-level error).
const ErrorState = ({ error, onRetry, retrying = false, className }: ErrorStateProps) => {
  const p = toProblem(error);
  return (
    <Card
      alignItems="center"
      borderRadius={8}
      className={className}
      flexShrink={0}
      gap={12}
      justifyContent="center"
      // A caller's `className` sets its own padding, so the default 32 stands only without one.
      {...(className ? {} : { padding: 32 })}
      role="alert"
      tone="onair"
    >
      <Icon decorative glyph={icons.alert} size="control" tone="danger" />
      <div>
        <Text as="p" textAlign="center" textRole="cardLabel" tone="danger">
          {p.title ?? "Something went wrong"}
        </Text>
        {p.detail && (
          <Stack marginTop={4} maxWidth={448}>
            <Text as="p" textAlign="center" textRole="compact" tone="secondary">
              {p.detail}
            </Text>
          </Stack>
        )}
      </div>
      {onRetry && (
        <Action onPress={retrying ? () => undefined : onRetry} variant="outline">
          {retrying ? "Trying again…" : "Try again"}
        </Action>
      )}
    </Card>
  );
};

export { ErrorState };
