import { toProblem } from "@loomarr/api/mutator";
import { AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";
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
      <AlertTriangle className="text-onair-300 size-6" aria-hidden />
      <div>
        <p className="text-onair-300 font-medium">{p.title ?? "Something went wrong"}</p>
        {p.detail && <p className="text-muted-foreground mt-1 max-w-md text-sm">{p.detail}</p>}
      </div>
      {onRetry && (
        <Button variant="outline" size="sm" onClick={onRetry}>
          Try again
        </Button>
      )}
    </div>
  );
};

export { ErrorState };
