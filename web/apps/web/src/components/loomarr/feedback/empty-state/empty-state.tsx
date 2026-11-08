import { Action } from "@loomarr/design-system";
import { cn } from "@/lib/utils";
import type { EmptyStateProps } from "./empty-state.type";

// EmptyState — mandatory for every list (§6): a calm surface with exactly ONE next
// action and on-theme microcopy ("Dead air — create your first channel"). Idle
// surface, so a CRT flourish is allowed here (§1) — added later, off in
// visual-test mode.
const EmptyState = ({ icon, title, description, action, className }: EmptyStateProps) => (
  <div className={cn("flex flex-col items-center justify-center gap-3 p-10 text-center", className)}>
    {icon && <div className="text-static-500 [&_svg]:size-8">{icon}</div>}
    <div>
      <p className="text-static-400 text-sm">{title}</p>
      {description && <p className="text-muted-foreground mt-1 max-w-sm text-sm">{description}</p>}
    </div>
    {action && <Action onPress={action.onClick}>{action.label}</Action>}
  </div>
);

export { EmptyState };
