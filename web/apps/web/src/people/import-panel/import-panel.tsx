import * as settingsApi from "@loomarr/api/endpoints/settings";
import * as usersApi from "@loomarr/api/endpoints/users";
import { unwrap } from "@loomarr/api/unwrap";
import { pluralize } from "@loomarr/core/format";
import { useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import type { ImportPanelProps } from "./import-panel.type";

// ImportPanel — §11's "explicit import, never implicit". A media-server account grants no
// access until an admin picks it here; signing in is not self-provisioning. The list shows
// real names rather than asking for pasted ids, and already-imported accounts stay visible
// (checked and locked) so "why isn't Ada listed?" never becomes a question.
const ImportPanel = ({ onImported, className }: ImportPanelProps) => {
  const [picked, setPicked] = useState<Set<string>>(new Set());

  // The route stays registered, while the computed feature set prevents offering an
  // operation that the live media-server connection cannot currently perform.
  const settings = settingsApi.useSettingsList();
  const available = settings.data?.status === 200 ? Boolean(settings.data.data.features?.user_sync) : false;

  const candidates = usersApi.useImportCandidates({ query: { enabled: available } });
  const importUsers = usersApi.useImportUsers({
    mutation: {
      onSuccess: () => {
        setPicked(new Set());
        onImported?.();
      },
    },
  });
  const sync = usersApi.useSyncUsers({ mutation: { onSuccess: () => onImported?.() } });

  if (!available) {
    return (
      <Card className={className}>
        <div className="p-4">
          <h2 className="font-semibold text-lg">Import from your media server</h2>
          {/* No media server means this is not a permissions problem or a missing key —
              there is nothing to import FROM. Say that, rather than showing a dead
              button that would 404. */}
          <p className="mt-1 text-muted-foreground text-sm">
            Connect Emby or Jellyfin under Settings → Connections, and their accounts can be imported here.
            Until then, the only account is the one you created during setup.
          </p>
        </div>
      </Card>
    );
  }

  const rows = unwrap(candidates.data, (b) => b.candidates) ?? [];
  const importable = rows.filter((c) => !c.imported);

  const toggle = (id: string) =>
    setPicked((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  return (
    <Card className={className}>
      <div className="flex flex-wrap items-start justify-between gap-3 p-4">
        <div>
          <h2 className="font-semibold text-lg">Import from your media server</h2>
          <p className="mt-1 text-muted-foreground text-sm">
            Pick who may sign in. Their existing media-server password is verified there; after a successful
            sign-in Loomarr stores only a non-reversible verifier for outage access. Server admins are
            imported as Loomarr admins initially.
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          disabled={sync.isPending}
          onClick={() => sync.mutate()}
          // Sync refreshes names and disabled state for accounts ALREADY imported; it
          // never adds anyone (§11). Saying so prevents "I synced, why is nobody here?"
          title="Refresh names and disabled state for accounts already imported"
        >
          {sync.isPending ? "Syncing…" : "Sync existing"}
        </Button>
      </div>

      {candidates.error != null && <ErrorState error={candidates.error} />}
      {importUsers.error != null && <ErrorState error={importUsers.error} />}
      {sync.error != null && <ErrorState error={sync.error} />}

      {candidates.isLoading ? (
        <p className="px-4 pb-4 text-muted-foreground text-sm">Loading accounts…</p>
      ) : rows.length === 0 ? (
        <p className="px-4 pb-4 text-muted-foreground text-sm">Your media server reports no accounts.</p>
      ) : (
        <>
          <ul className="border-static-800 border-t">
            {rows.map((c) => {
              const id = `cand-${c.id}`;
              return (
                <li
                  key={c.id}
                  className="flex items-center gap-3 border-static-800 border-b px-4 py-2 last:border-b-0"
                >
                  <Checkbox
                    id={id}
                    // An imported account renders checked and locked: hiding it would
                    // read as "missing", and offering it again would imply a no-op
                    // action does something.
                    checked={c.imported || picked.has(c.id)}
                    disabled={c.imported}
                    onChange={() => toggle(c.id)}
                  />
                  <Label htmlFor={id} className="flex flex-1 items-center gap-2 font-normal">
                    <span>{c.name}</span>
                    {c.isAdmin && <span className="text-static-400 text-xs">server admin</span>}
                    {c.disabled && <span className="text-onair-300 text-xs">disabled on server</span>}
                    {c.imported && <span className="text-lock text-xs">already imported</span>}
                  </Label>
                </li>
              );
            })}
          </ul>

          <div className="flex justify-end p-4">
            <Button
              size="sm"
              disabled={picked.size === 0 || importUsers.isPending}
              onClick={() => importUsers.mutate({ data: { ids: [...picked] } })}
            >
              {importUsers.isPending ? "Importing…" : `Import ${pluralize(picked.size, "account")}`}
            </Button>
          </div>

          {importable.length === 0 && (
            <p className="px-4 pb-4 text-muted-foreground text-sm">
              Every media-server account is already imported.
            </p>
          )}
        </>
      )}
    </Card>
  );
};

export { ImportPanel };
