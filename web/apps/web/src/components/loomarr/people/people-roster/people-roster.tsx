import { KeyRound, Search, Server } from "lucide-react";
import { useId, useMemo, useState } from "react";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { cn } from "@/lib/utils";
import type { PeopleRosterProps } from "./people-roster.type";

const credentialLabel = (local: boolean, offlineLogin: boolean) => {
  if (local) return "Local account";
  return offlineLogin
    ? "Media-server account · offline login ready"
    : "Media-server account · sign in once to enable offline login";
};

const PeopleRoster = ({ users, selectedId, selfId, onSelect }: PeopleRosterProps) => {
  const searchId = useId();
  const roleId = useId();
  const statusId = useId();
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
          <h2 id="people-roster-title" className="font-semibold text-lg">
            People
          </h2>
          <p className="text-muted-foreground text-sm">
            Select a person to manage access, requests, and sessions.
          </p>
        </div>
        <div className="grid gap-3 sm:grid-cols-[minmax(12rem,1fr)_9rem_9rem] lg:w-auto">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor={searchId}>Search people</Label>
            <div className="relative">
              <Search
                className="pointer-events-none absolute top-2.5 left-3 size-4 text-muted-foreground"
                aria-hidden
              />
              <Input
                id={searchId}
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                className="pl-9"
                placeholder="Name"
              />
            </div>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor={roleId}>Role</Label>
            <Select value={role} onValueChange={(value) => setRole(String(value))}>
              <SelectTrigger id={roleId}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All roles</SelectItem>
                <SelectItem value="admin">Admin</SelectItem>
                <SelectItem value="member">Member</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor={statusId}>Status</Label>
            <Select value={status} onValueChange={(value) => setStatus(String(value))}>
              <SelectTrigger id={statusId}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All statuses</SelectItem>
                <SelectItem value="enabled">Enabled</SelectItem>
                <SelectItem value="disabled">Disabled</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>
      </div>

      {users === undefined ? (
        <div className="grid gap-2" role="status" aria-label="Loading people">
          {[0, 1, 2].map((item) => (
            <div key={item} className="h-20 animate-pulse rounded-lg border border-border bg-card" />
          ))}
        </div>
      ) : users.length === 0 ? (
        <EmptyState
          title="No people yet"
          description="Import a media-server account or create a local account below."
        />
      ) : filtered.length === 0 ? (
        <div className="rounded-lg border border-border bg-card p-6 text-center">
          <p className="font-medium">No matching people</p>
          <p className="text-muted-foreground text-sm">Try another name, role, or status.</p>
        </div>
      ) : (
        <div className="overflow-hidden rounded-lg border border-border bg-card">
          <div
            className="hidden grid-cols-[minmax(0,2fr)_8rem_8rem_8rem] gap-4 border-static-800 border-b px-4 py-2 font-mono text-static-400 text-xs uppercase tracking-wide md:grid"
            aria-hidden
          >
            <span>Person</span>
            <span>Role</span>
            <span>Requests</span>
            <span>Status</span>
          </div>
          <ul>
            {filtered.map((user) => (
              <li key={user.id} className="border-static-800 border-b last:border-b-0">
                <button
                  type="button"
                  aria-label={`Manage ${user.name}`}
                  aria-current={selectedId === user.id ? "true" : undefined}
                  onClick={() => onSelect(user)}
                  className={cn(
                    "grid w-full cursor-pointer grid-cols-1 gap-3 p-4 text-left transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset md:grid-cols-[minmax(0,2fr)_8rem_8rem_8rem] md:items-center",
                    user.disabled && "bg-static-900/40",
                    selectedId === user.id && "bg-accent",
                  )}
                >
                  <span className="min-w-0">
                    <span className="flex flex-wrap items-center gap-2">
                      <span className="truncate font-medium">{user.name}</span>
                      {selfId === user.id && <Badge variant="tune">You</Badge>}
                    </span>
                    <span className="mt-1 flex items-center gap-1.5 text-static-400 text-xs">
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
                      <Badge variant={user.role === "admin" ? "tune" : "neutral"}>{user.role}</Badge>
                  </span>
                  <span className="flex items-center justify-between gap-3 font-mono text-sm tabular-nums md:block">
                    <span className="font-sans text-muted-foreground text-xs md:hidden">Requests</span>
                    {`${user.pendingAcquisitions} / ${user.effectiveQuota}`}
                  </span>
                  <span className="flex items-center justify-between gap-3 md:block">
                    <span className="text-muted-foreground text-xs md:hidden">Status</span>
                    {user.disabled ? <Badge variant="onair">Disabled</Badge> : <Badge>Enabled</Badge>}
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
