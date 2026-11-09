import { toProblem } from "@loomarr/api/mutator";
import { Action, Icon, icons } from "@loomarr/design-system";
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
        <p className="text-onair-300 font-medium">{p.title ?? "Something went wrong"}</p>
        {p.detail && <p className="text-muted-foreground mt-1 max-w-md text-sm">{p.detail}</p>}
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
