import { Link } from "@tanstack/react-router";
import { ChevronRight } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import type { RequestCardProps } from "./request-card.type";

// RequestCard — one request on the Requests page (#1405). Everything but the optional `action` is
// the link to the request's detail: the card used to be an inert box, so a member who clicked a
// request saw nothing happen. It carries one thing beyond what was asked — a single status line —
// because the page's question is "where does each of my requests stand?"; the detail answers the
// rest.
//
// ⚠ The action is a SIBLING of the link, not a child: a button or link nested inside a link is
// invalid HTML and makes both controls unreliable for keyboard and assistive-tech users.
const RequestCard = ({ jobId, title, line, tone, createdAt, hint, action, className }: RequestCardProps) => (
  <Card className={cn("flex items-center gap-3 transition-colors hover:bg-accent", className)}>
    <Link
      to="/requests/$jobId"
      params={{ jobId }}
      className="flex min-w-0 flex-1 items-center gap-3 rounded-lg p-4 outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <div className="flex min-w-0 flex-1 flex-col gap-1.5">
        <p className="font-medium">{title}</p>
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant={tone}>{line}</Badge>
          <span className="text-muted-foreground text-xs">
            Requested {new Date(createdAt).toLocaleDateString()}
          </span>
        </div>
        {hint && <p className="text-muted-foreground text-sm">{hint}</p>}
      </div>
      <ChevronRight className="size-4 shrink-0 text-muted-foreground" aria-hidden />
    </Link>
    {action && <div className="shrink-0 pr-4">{action}</div>}
  </Card>
);

export { RequestCard };
