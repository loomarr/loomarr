import { usersApi } from "@loomarr/api";
import { Loader2 } from "lucide-react";
import { useState } from "react";
import { EmptyState, ErrorState } from "@/components/loomarr";
import { Button, Checkbox, Label } from "@/components/ui";

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

  const rows = candidates.data?.status === 200 ? (candidates.data.data.candidates ?? []) : [];
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
      <div className="flex flex-col gap-3">
        <ErrorState error={candidates.error} onRetry={() => candidates.refetch()} />
        <p className="text-muted-foreground text-sm">
          You can skip this and import people later from Settings → Users.
        </p>
      </div>
    );
  }

  if (candidates.isLoading) {
    return (
      <p className="flex items-center gap-2 text-muted-foreground text-sm">
        <Loader2 className="size-4 animate-spin" aria-hidden />
        Looking up your media-server accounts…
      </p>
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
      <p className="text-muted-foreground text-sm">
        Only the people you pick can sign in. Everyone else is ignored, even with valid media-server
        credentials.
      </p>

      <ul className="flex flex-col gap-2">
        {rows.map((c) => {
          const id = `candidate-${c.id}`;
          return (
            <li
              key={c.id}
              className="flex items-center gap-3 rounded-md border border-border bg-card px-3 py-2.5"
            >
              <Checkbox
                id={id}
                checked={c.imported || selected.has(c.id)}
                disabled={c.imported}
                onChange={() => toggle(c.id)}
              />
              <Label htmlFor={id} className="flex-1 cursor-pointer font-normal">
                {c.name}
              </Label>
              {c.isAdmin && <span className="text-static-400 text-xs">server admin</span>}
              {c.imported && <span className="text-lock text-xs">imported</span>}
            </li>
          );
        })}
      </ul>

      {importUsers.error != null && <ErrorState error={importUsers.error} />}

      <Button
        className="w-fit"
        disabled={selected.size === 0 || importUsers.isPending}
        onClick={() =>
          importUsers.mutate({ data: { ids: [...selected] } }, { onSuccess: () => setSelected(new Set()) })
        }
      >
        {importUsers.isPending && <Loader2 className="animate-spin" aria-hidden />}
        {importUsers.isPending
          ? "Importing…"
          : `Import ${selected.size || ""} ${selected.size === 1 ? "person" : "people"}`.trim()}
      </Button>

      {importable.length === 0 && (
        <p className="text-muted-foreground text-xs">Everyone on your media server is imported.</p>
      )}
    </div>
  );
};

export { UsersStep };
