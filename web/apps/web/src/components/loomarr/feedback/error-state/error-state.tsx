import { toProblem } from "@loomarr/api/mutator";
import { Action, Icon, icons, Text } from "@loomarr/design-system";
import { cn } from "@/lib/utils";
import type { ErrorStateProps } from "./error-state.type";

// ErrorState — the RFC 7807 renderer (§3). Every failure surfaces the problem's
// `title`/`detail` as words, never raw JSON (§6). Retry is offered only when the
// caller says the operation is idempotent. Field-level 422 errors render inline via
// TanStack Form instead (this is the block/list-level error).
const ErrorState = ({ error, onRetry, className }: ErrorStateProps) => {
  const p = toProblem(error);
  return (
    <div
      role="alert"
      className={cn(
        "border-onair-tint-30 bg-onair-tint-8 flex flex-col items-center justify-center gap-3 rounded-lg border p-8 text-center",
        className,
      )}
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
        <Action onPress={onRetry} variant="outline">
          Try again
        </Action>
      )}
    </div>
  );
};

export { ErrorState };
