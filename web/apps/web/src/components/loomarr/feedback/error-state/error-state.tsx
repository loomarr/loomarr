import { toProblem } from "@loomarr/api/mutator";
import { Action, Icon, icons, Stack, Text } from "@loomarr/design-system";
import { cn } from "@/lib/utils";
import type { ErrorStateProps } from "./error-state.type";

// ErrorState — the RFC 7807 renderer (§3). Every failure surfaces the problem's
// `title`/`detail` as words, never raw JSON (§6). Retry is offered only when the
// caller says the operation is idempotent. Field-level 422 errors render inline via
// TanStack Form instead (this is the block/list-level error).
const ErrorState = ({ error, onRetry, retrying = false, className }: ErrorStateProps) => {
  const p = toProblem(error);
  return (
    <Stack
      align="center"
      className={cn("border-onair-tint-30 bg-onair-tint-8 rounded-lg border p-8 text-center", className)}
      flexShrink={0}
      gap="control"
      justify="center"
      role="alert"
    >
      <Icon decorative glyph={icons.alert} size="control" tone="danger" />
      <div>
        <Text as="p" textRole="cardLabel" tone="danger">
          {p.title ?? "Something went wrong"}
        </Text>
        {p.detail && (
          <Text as="p" className="mt-1 max-w-md" textRole="compact" tone="secondary">
            {p.detail}
          </Text>
        )}
      </div>
      {onRetry && (
        <Action onPress={retrying ? () => undefined : onRetry} variant="outline">
          {retrying ? "Trying again…" : "Try again"}
        </Action>
      )}
    </Stack>
  );
};

export { ErrorState };
