import * as usersApi from "@loomarr/api/endpoints/users";
import { unwrap } from "@loomarr/api/unwrap";
import { Action, LoadingStrip, Stack, Toggle } from "@loomarr/design-system";
import { useState } from "react";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { ErrorState } from "@/components/loomarr/feedback/error-state";

// Wizard step 6 — import media-server users (§11, §13). A media-server account grants
// NO access until an admin explicitly imports it: signing in is not self-provisioning.
// So this is a real allowlist decision, and it's skippable — a solo install needs only
// the bootstrap admin. Already-imported accounts stay visible but locked, because
// "where did that person go?" is worse than a disabled row.
const UsersStep = () => {
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const candidates = usersApi.useImportCandidates({ query: { retry: false } });
  const importUsers = usersApi.useImportUsers({
    mutation: { onSuccess: () => candidates.refetch() },
  });

  const rows = unwrap(candidates.data, (b) => b.candidates) ?? [];
  const importable = rows.filter((c) => !c.imported);

  const toggle = (id: string) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  if (candidates.error) {
    // Most often "no media server configured" — that's a reason to skip, not a failure
    // to agonize over, so the copy points at the exit.
    return (
      <Stack gap="control">
        <ErrorState error={candidates.error} onRetry={() => candidates.refetch()} />
        <p className="text-muted-foreground text-sm">
          You can skip this and import people later from Settings → Users.
        </p>
      </Stack>
    );
  }

  if (candidates.isLoading) {
    return (
      <Stack align="center" className="text-muted-foreground text-sm" direction="row" gap="inline" render="p">
        <LoadingStrip decorative width={32} />
        Looking up your media-server accounts…
      </Stack>
    );
  }

  if (rows.length === 0) {
    return (
      <EmptyState
        title="No accounts found"
        description="Your media server reported no users to import. You can skip this step."
      />
    );
  }

  return (
    <div className="flex flex-col gap-4">
      {/* The shell's step description already covers "only the accounts you pick can sign
          in"; this adds the part that surprises people (§11) rather than repeating it. */}
      <p className="text-muted-foreground text-sm">
        Anyone you don't import is refused even with correct media-server credentials.
      </p>

      <Stack gap="inline" render="ul">
        {rows.map((c) => {
          const id = `candidate-${c.id}`;
          return (
            <Stack
              align="center"
              className="border-border bg-card rounded-md border px-3 py-2.5"
              direction="row"
              gap="control"
              key={c.id}
              render="li"
            >
              <div className="flex-1">
                <Toggle
                  id={id}
                  label={c.name}
                  checked={c.imported || selected.has(c.id)}
                  disabled={c.imported}
                  onCheckedChange={() => toggle(c.id)}
                />
              </div>
              {c.isAdmin && <span className="text-static-400 text-xs">server admin</span>}
              {c.imported && <span className="text-lock text-xs">imported</span>}
            </Stack>
          );
        })}
      </Stack>

      {importUsers.error != null && <ErrorState error={importUsers.error} />}

      <Action
        disabled={selected.size === 0 || importUsers.isPending}
        onPress={() =>
          importUsers.mutate({ data: { ids: [...selected] } }, { onSuccess: () => setSelected(new Set()) })
        }
        style={{ alignSelf: "flex-start" }}
      >
        {importUsers.isPending
          ? "Importing…"
          : `Import ${selected.size || ""} ${selected.size === 1 ? "person" : "people"}`.trim()}
      </Action>

      {importable.length === 0 && (
        <p className="text-muted-foreground text-xs">Everyone on your media server is imported.</p>
      )}
    </div>
  );
};

export { UsersStep };
