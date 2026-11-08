import { UserBodyRole } from "@loomarr/api/models/userBodyRole";
import { formatRelative } from "@loomarr/core/format";
import { Text } from "@loomarr/design-system";
import { CardAction, CardStatus, ListGroup, RowCard, SearchField } from "@loomarr/ui";
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
        <ListGroup aria-labelledby="people-roster-title">
          {filtered.map((user) => (
            <div
              key={user.id}
              aria-current={selectedId === user.id ? "true" : undefined}
              className={cn(selectedId === user.id && "bg-accent")}
            >
              <RowCard
                action={
                  <CardAction accessibilityLabel={`Manage ${user.name}`} onPress={() => onSelect(user)}>
                    Manage
                  </CardAction>
                }
                lead={<Text textRole="cardInitials">{user.name.slice(0, 1).toUpperCase()}</Text>}
                meta={[
                  user.disabled ? (
                    <CardStatus key="status" label="Disabled" tone="off" />
                  ) : (
                    <CardStatus key="status" label={selfId === user.id ? "You" : "Active"} tone="ok" />
                  ),
                  user.role === UserBodyRole.admin ? "Admin" : "Member",
                  credentialLabel(user.local, user.offlineLogin),
                  // The API leaves last seen out for someone who has never signed in (#1667).
                  user.lastSeenAt === undefined
                    ? "Never seen"
                    : `seen ${formatRelative(user.lastSeenAt, now)}`,
                ]}
                title={user.name}
              />
            </div>
          ))}
        </ListGroup>
      )}
    </section>
  );
};

export { PeopleRoster };
