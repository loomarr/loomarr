import {
  Action,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Icon,
  icons,
  StatusLabel,
  Toggle,
} from "@loomarr/design-system";
import { SearchField } from "@loomarr/ui";
import { useId, useMemo, useState } from "react";
import { BrowserSelectControl } from "@/client-adapters";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import type { ImportDialogProps } from "./import-dialog.type";

type CandidateFilter = "all" | "available" | "imported" | "disabled" | "admin";

const CANDIDATE_FILTERS: { label: string; value: CandidateFilter }[] = [
  { label: "All accounts", value: "all" },
  { label: "Available to import", value: "available" },
  { label: "Already imported", value: "imported" },
  { label: "Disabled on server", value: "disabled" },
  { label: "Server admins", value: "admin" },
];

const ImportDialog = ({
  available,
  candidates,
  candidateError,
  mutationError,
  importing = false,
  syncing = false,
  defaultOpen = false,
  portalContainer,
  onRetry,
  onImport,
  onSync,
}: ImportDialogProps) => {
  const searchId = useId();
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<CandidateFilter>("all");
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const [open, setOpen] = useState(defaultOpen);

  const visible = useMemo(() => {
    const term = query.trim().toLocaleLowerCase();
    return (candidates ?? []).filter((candidate) => {
      if (term && !candidate.name.toLocaleLowerCase().includes(term)) return false;
      if (filter === "available") return !candidate.imported;
      if (filter === "imported") return candidate.imported;
      if (filter === "disabled") return candidate.disabled;
      if (filter === "admin") return candidate.isAdmin;
      return true;
    });
  }, [candidates, filter, query]);

  const visibleImportable = visible.filter((candidate) => !candidate.imported);
  const selectedIds = [...picked].filter((id) =>
    candidates?.some((candidate) => candidate.id === id && !candidate.imported),
  );
  const allVisiblePicked =
    visibleImportable.length > 0 && visibleImportable.every((candidate) => picked.has(candidate.id));

  const toggle = (id: string) =>
    setPicked((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  const toggleVisible = () =>
    setPicked((current) => {
      const next = new Set(current);
      for (const candidate of visibleImportable) {
        if (allVisiblePicked) next.delete(candidate.id);
        else next.add(candidate.id);
      }
      return next;
    });

  const importSelected = async () => {
    try {
      await onImport(selectedIds);
      setPicked(new Set());
    } catch {
      // The connected caller exposes the RFC 7807 problem through `error`; retaining the
      // selection makes a transient provider failure retryable without rebuilding the batch.
    }
  };

  const syncExisting = async () => {
    try {
      await onSync();
    } catch {
      // See importSelected: the problem stays visible in the focused workflow.
    }
  };

  // The dialog opens from its own state rather than a trigger part, so the opener can be an Action;
  // the Dialog returns focus to it on close all the same.
  return (
    <>
      <Action onPress={() => setOpen(true)}>Import from Emby/Jellyfin</Action>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent layout="sectioned" portalContainer={portalContainer} size="reading">
          <DialogHeader>
            <DialogTitle>Import from Emby/Jellyfin</DialogTitle>
            <DialogDescription>
              Choose which media-server accounts may sign in to Loomarr. Nothing is provisioned unless you
              explicitly select it.
            </DialogDescription>
          </DialogHeader>

          {!available ? (
            <div className="flex flex-col items-start gap-4 overflow-auto px-6 pb-6">
              <div className="border-border bg-static-900 rounded-lg border p-4">
                <p className="font-medium">Connect a media server first</p>
                <p className="text-muted-foreground mt-1 text-sm">
                  Add Emby or Jellyfin under Connections. Once Loomarr can reach it, its accounts will be
                  available here for explicit import.
                </p>
              </div>
              <Action accessibilityRole="link" render={<a href="/settings/connections" />} variant="outline">
                Open Connections settings
              </Action>
            </div>
          ) : (
            <>
              <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-auto px-6">
                <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_12rem]">
                  <SearchField
                    label="Search accounts"
                    onChangeText={setQuery}
                    placeholder="Name"
                    value={query}
                  />
                  <BrowserSelectControl
                    appearance="field"
                    label="Show"
                    onValueChange={setFilter}
                    options={CANDIDATE_FILTERS}
                    value={filter}
                  />
                </div>

                {mutationError != null && <ErrorState error={mutationError} className="p-4" />}

                {candidateError != null ? (
                  <ErrorState error={candidateError} onRetry={onRetry} className="p-4" />
                ) : candidates === undefined ? (
                  <div className="grid gap-2" role="status" aria-label="Loading media-server accounts">
                    {[0, 1, 2].map((item) => (
                      <div
                        key={item}
                        className="border-border bg-card h-16 animate-pulse rounded-lg border"
                      />
                    ))}
                  </div>
                ) : candidates.length === 0 ? (
                  <EmptyState
                    title="No accounts reported"
                    description="Emby/Jellyfin did not return any accounts to import."
                  />
                ) : visible.length === 0 ? (
                  <div className="border-border bg-card rounded-lg border p-6 text-center">
                    <p className="font-medium">No matching accounts</p>
                    <p className="text-muted-foreground text-sm">Try another name or account filter.</p>
                  </div>
                ) : (
                  <div className="border-border bg-card overflow-hidden rounded-lg border">
                    <div className="border-static-800 flex flex-wrap items-center justify-between gap-2 border-b px-4 py-2">
                      <p aria-live="polite" className="text-muted-foreground text-sm">
                        {selectedIds.length} selected · {visibleImportable.length} importable shown
                      </p>
                      <Action
                        disabled={visibleImportable.length === 0 || importing}
                        onPress={toggleVisible}
                        variant="ghost"
                      >
                        {allVisiblePicked ? "Clear visible" : "Select visible"}
                      </Action>
                    </div>
                    <ul>
                      {visible.map((candidate) => {
                        const id = `${searchId}-${candidate.id}`;
                        return (
                          <li
                            key={candidate.id}
                            className="border-static-800 flex items-start gap-3 border-b px-4 py-1 last:border-b-0"
                          >
                            <Toggle
                              id={id}
                              label={candidate.name}
                              labelHidden
                              checked={candidate.imported || picked.has(candidate.id)}
                              disabled={candidate.imported || importing}
                              onCheckedChange={() => toggle(candidate.id)}
                            />
                            <label
                              htmlFor={id}
                              className="min-w-0 flex-1 cursor-pointer py-2 text-sm leading-none"
                            >
                              <span className="flex flex-wrap items-center gap-2">
                                <span className="font-medium">{candidate.name}</span>
                                {candidate.imported && <StatusLabel tone="ok">Already imported</StatusLabel>}
                                {candidate.disabled && (
                                  <StatusLabel tone="error">Disabled on server</StatusLabel>
                                )}
                                {candidate.isAdmin && <StatusLabel tone="notice">Server admin</StatusLabel>}
                              </span>
                              <span className="text-static-400 mt-1 block text-xs">
                                Initial Loomarr role: {candidate.isAdmin ? "Admin" : "Member"}
                              </span>
                            </label>
                          </li>
                        );
                      })}
                    </ul>
                  </div>
                )}

                <div className="border-border bg-static-900 grid gap-3 rounded-lg border p-4 text-sm sm:grid-cols-2">
                  <div>
                    <p className="flex items-center gap-2 font-medium">
                      <Icon decorative glyph={icons.verified} size="compact" tone="success" />
                      Passwords stay private
                    </p>
                    <p className="text-muted-foreground mt-1">
                      Emby/Jellyfin validates the password. After a successful sign-in, Loomarr stores only a
                      non-reversible Argon2id verifier for outage access and refreshes it after later provider
                      sign-ins.
                    </p>
                  </div>
                  <div>
                    <p className="font-medium">Sync existing accounts</p>
                    <p className="text-muted-foreground mt-1">
                      Refresh imported names and disabled state. Sync never provisions an unselected account.
                    </p>
                    <div className="mt-3 flex">
                      <Action
                        disabled={syncing || importing}
                        onPress={() => void syncExisting()}
                        variant="outline"
                      >
                        {syncing ? "Syncing…" : "Sync existing"}
                      </Action>
                    </div>
                  </div>
                </div>
              </div>

              <DialogFooter>
                <Action
                  disabled={selectedIds.length === 0 || importing || syncing}
                  onPress={() => void importSelected()}
                >
                  {importing
                    ? "Importing…"
                    : `Import ${selectedIds.length} ${selectedIds.length === 1 ? "account" : "accounts"}`}
                </Action>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </>
  );
};

export { ImportDialog };
