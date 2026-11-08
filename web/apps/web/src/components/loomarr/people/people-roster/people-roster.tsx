import { UserBodyRole } from "@loomarr/api/models/userBodyRole";
import { formatRelative } from "@loomarr/core/format";
import { Badge } from "@loomarr/design-system";
import { SearchField } from "@loomarr/ui";
import { KeyRound, Server } from "lucide-react";
import { useMemo, useState } from "react";
import { BrowserSelectControl } from "@/client-adapters";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { cn } from "@/lib/utils";
import type { PeopleRosterProps } from "./people-roster.type";

const ROLE_FILTERS = [
  { label: "All roles", value: "all" },
  { label: "Admin", value: UserBodyRole.admin },
  { label: "Member", value: UserBodyRole.member },
];

const STATUS_FILTERS = [
  { label: "All statuses", value: "all" },
  { label: "Enabled", value: "enabled" },
  { label: "Disabled", value: "disabled" },
];

const credentialLabel = (local: boolean, offlineLogin: boolean) => {
  if (local) return "Local account";
  return offlineLogin
    ? "Media-server account · offline login ready"
    : "Media-server account · sign in once to enable offline login";
};

const PeopleRoster = ({ users, selectedId, selfId, onSelect, now = Date.now() }: PeopleRosterProps) => {
  const [query, setQuery] = useState("");
  const [role, setRole] = useState("all");
  const [status, setStatus] = useState("all");

  const filtered = useMemo(() => {
    const term = query.trim().toLocaleLowerCase();
    return (users ?? []).filter(
      (user) =>
        (!term || user.name.toLocaleLowerCase().includes(term)) &&
        (role === "all" || user.role === role) &&
        (status === "all" || (status === "disabled") === user.disabled),
    );
  }, [query, role, status, users]);

  return (
    <section aria-labelledby="people-roster-title" className="flex flex-col gap-3">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-end">
        <div className="min-w-0 flex-1">
          <h2 id="people-roster-title" className="text-lg font-semibold">
            People
          </h2>
          <p className="text-muted-foreground text-sm">
            Select a person to manage access, requests, and sessions.
          </p>
        </div>
        <div className="grid gap-3 sm:grid-cols-[minmax(12rem,1fr)_9rem_9rem] lg:w-auto">
          <SearchField label="Search people" onChangeText={setQuery} placeholder="Name" value={query} />
          <BrowserSelectControl
            appearance="field"
            label="Role"
            onValueChange={setRole}
            options={ROLE_FILTERS}
            value={role}
          />
          <BrowserSelectControl
            appearance="field"
            label="Status"
            onValueChange={setStatus}
            options={STATUS_FILTERS}
            value={status}
          />
        </div>
      </div>

      {users === undefined ? (
        <div className="grid gap-2" role="status" aria-label="Loading people">
          {[0, 1, 2].map((item) => (
            <div key={item} className="border-border bg-card h-20 animate-pulse rounded-lg border" />
          ))}
        </div>
      ) : users.length === 0 ? (
        <EmptyState
          title="No people yet"
          description="Use the actions above to import a media-server account or add a local one."
        />
      ) : filtered.length === 0 ? (
        <div className="border-border bg-card rounded-lg border p-6 text-center">
          <p className="font-medium">No matching people</p>
          <p className="text-muted-foreground text-sm">Try another name, role, or status.</p>
        </div>
      ) : (
        <div className="border-border bg-card overflow-hidden rounded-lg border">
          <div
            className="border-static-800 text-static-400 hidden grid-cols-[minmax(0,2fr)_8rem_8rem_8rem_8rem] gap-4 border-b px-4 py-2 font-mono text-xs tracking-wide uppercase md:grid"
            aria-hidden
          >
            <span>Person</span>
            <span>Role</span>
            <span>Request quota</span>
            <span>Status</span>
            <span>Last seen</span>
          </div>
          <ul>
            {filtered.map((user) => (
              <li key={user.id} className="border-static-800 border-b last:border-b-0">
                <button
                  type="button"
                  aria-current={selectedId === user.id ? "true" : undefined}
                  onClick={() => onSelect(user)}
                  className={cn(
                    "hover:bg-accent focus-visible:ring-ring relative grid w-full cursor-pointer grid-cols-1 gap-3 p-4 text-left transition-colors focus-visible:ring-2 focus-visible:outline-none focus-visible:ring-inset md:grid-cols-[minmax(0,2fr)_8rem_8rem_8rem_8rem] md:items-center",
                    user.disabled && "bg-static-900/40",
                    selectedId === user.id && "bg-accent",
                  )}
                >
                  {/* The name comes from the content, so it holds every visible word (WCAG 2.5.3). */}
                  <span className="sr-only">Manage</span>{" "}
                  <span className="min-w-0">
                    <span className="flex flex-wrap items-center gap-2">
                      <span className="truncate font-medium">{user.name}</span>
                      {selfId === user.id && (
                        <Badge shape="square" tone="tune">
                          You
                        </Badge>
                      )}
                    </span>
                    <span className="text-static-400 mt-1 flex items-center gap-1.5 text-xs">
                      {user.local ? (
                        <KeyRound className="size-3" aria-hidden />
                      ) : (
                        <Server className="size-3" aria-hidden />
                      )}
                      {credentialLabel(user.local, user.offlineLogin)}
                    </span>
                  </span>
                  <span className="flex items-center justify-between gap-3 md:block">
                    <span className="text-muted-foreground text-xs md:hidden">Role</span>
                    <Badge shape="square" tone={user.role === "admin" ? "tune" : "neutral"}>
                      {user.role}
                    </Badge>
                  </span>
                  <span className="flex items-center justify-between gap-3 font-mono text-sm tabular-nums md:block">
                    <span className="text-muted-foreground font-sans text-xs md:hidden">Request quota</span>
                    {`${user.pendingAcquisitions} / ${user.effectiveQuota}`}
                  </span>
                  <span className="flex items-center justify-between gap-3 md:block">
                    <span className="text-muted-foreground text-xs md:hidden">Status</span>
                    {user.disabled ? (
                      <Badge shape="square" tone="onair">
                        Disabled
                      </Badge>
                    ) : (
                      <Badge shape="square">Enabled</Badge>
                    )}
                  </span>
                  {/* The web mock's Last seen: mono, muted. The API leaves it out for someone who
                      has never signed in (#1667). */}
                  <span className="flex items-center justify-between gap-3 md:block">
                    <span className="text-muted-foreground text-xs md:hidden">Last seen</span>
                    <span className="text-muted-foreground font-mono text-xs">
                      {user.lastSeenAt === undefined ? "Never" : formatRelative(user.lastSeenAt, now)}
                    </span>
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
};

export { PeopleRoster };
