import { cn } from "@/lib/utils";
import { Progress } from "../progress";
import { StatusDot } from "../status-dot";
import type { ListGroupProps, ListRowProps } from "./list-row.type";

// ListGroup + ListRow — the bordered list of state rows (#1659 web mock: On the way, Your
// requests). A dot for what the row is waiting on, a title and sub line, and at most one of a
// progress readout or a trailing action.
//
// A real list (`ul`/`li`), so a screen reader announces "list, 3 items" before reading rows that
// otherwise only look like one.
//
// ⚠ The mock draws a hairline under EVERY row, the last included, which doubles against the
// group's own border. `last:border-b-0` drops that one line; nothing else departs from the mock.
const ListGroup = ({ children, className, ...aria }: ListGroupProps) => (
  <ul
    {...aria}
    className={cn(
      "m-0 list-none overflow-hidden rounded-lg border border-static-700 bg-static-900 p-0",
      className,
    )}
  >
    {children}
  </ul>
);

const ListRow = ({ tone, toneLabel = "", title, sub, progress, action, className }: ListRowProps) => (
  <li
    className={cn("flex items-center gap-4 border-static-700 border-b px-4 py-3 last:border-b-0", className)}
  >
    <StatusDot tone={tone} label={toneLabel} />
    <div className="min-w-0 flex-1">
      <p className="m-0 font-medium text-[13px]">{title}</p>
      {sub != null && <p className="mt-0.5 mb-0 text-static-400 text-xs">{sub}</p>}
    </div>
    {progress && (
      <div className="flex w-40 shrink-0 flex-col gap-1">
        <Progress value={progress.value} label={progress.label} tone="tune" className="h-1" />
        {progress.eta && (
          <span className="text-right font-mono text-2xs text-static-400">{progress.eta}</span>
        )}
      </div>
    )}
    {action}
  </li>
);

export { ListGroup, ListRow };
