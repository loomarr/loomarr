import {
  Action,
  Card,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Icon,
  icons,
  Stack,
  StatusLabel,
  Text,
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
      <Action onPress={() => setOpen(true)} variant="outline">
        Import from Emby/Jellyfin
      </Action>
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
            <Stack
              align="start"
              className="overflow-auto"
              flexShrink={1}
              gap="4"
              paddingBottom={24}
              paddingHorizontal={24}
            >
              <Card flexShrink={0} gap={4} padding={16}>
                <Text as="p" textRole="cardLabel">
                  Connect a media server first
                </Text>
                <Text as="p" textRole="compact" tone="secondary">
                  Add Emby or Jellyfin under Connections. Once Loomarr can reach it, its accounts will be
                  available here for explicit import.
                </Text>
              </Card>
              <Action accessibilityRole="link" render={<a href="/settings/connections" />} variant="outline">
                Open Connections settings
              </Action>
            </Stack>
          ) : (
            <>
              <Stack
                className="overflow-auto"
                flexBasis="0%"
                flexGrow={1}
                flexShrink={1}
                gap="4"
                minHeight={0}
                paddingHorizontal={24}
              >
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
                  <Stack aria-label="Loading media-server accounts" gap="2" role="status">
                    {[0, 1, 2].map((item) => (
                      <Card className="animate-pulse" flexShrink={0} height={64} key={item} />
                    ))}
                  </Stack>
                ) : candidates.length === 0 ? (
                  <EmptyState
                    title="No accounts reported"
                    description="Emby/Jellyfin did not return any accounts to import."
                  />
                ) : visible.length === 0 ? (
                  <Card alignItems="center" flexShrink={0} padding={24}>
                    <Text as="p" textRole="cardLabel">
                      No matching accounts
                    </Text>
                    <Text as="p" textRole="compact" tone="secondary">
                      Try another name or account filter.
                    </Text>
                  </Card>
                ) : (
                  <Card flexShrink={0} overflow="hidden">
                    <Stack
                      align="center"
                      borderBottomWidth={1}
                      borderColor="borderDecorative"
                      direction="row"
                      gap="inline"
                      justify="between"
                      paddingHorizontal={16}
                      paddingVertical={8}
                      wrap
                    >
                      <Text aria-live="polite" as="p" textRole="compact" tone="secondary">
                        {selectedIds.length} selected · {visibleImportable.length} importable shown
                      </Text>
                      <Action
                        disabled={visibleImportable.length === 0 || importing}
                        onPress={toggleVisible}
                        variant="ghost"
                      >
                        {allVisiblePicked ? "Clear visible" : "Select visible"}
                      </Action>
                    </Stack>
                    <ul>
                      {visible.map((candidate, index) => {
                        const id = `${searchId}-${candidate.id}`;
                        return (
                          <Stack
                            align="start"
                            borderBottomWidth={index === visible.length - 1 ? 0 : 1}
                            borderColor="borderDecorative"
                            direction="row"
                            gap="control"
                            key={candidate.id}
                            paddingHorizontal={16}
                            paddingVertical={4}
                            render="li"
                          >
                            <Toggle
                              id={id}
                              label={candidate.name}
                              labelHidden
                              checked={candidate.imported || picked.has(candidate.id)}
                              disabled={candidate.imported || importing}
                              onCheckedChange={() => toggle(candidate.id)}
                            />
                            <label htmlFor={id} className="min-w-0 flex-1 cursor-pointer py-2">
                              <Stack align="center" direction="row" gap="inline" render="span" wrap>
                                <Text as="span" textRole="cardLabel">
                                  {candidate.name}
                                </Text>
                                {candidate.imported && <StatusLabel tone="ok">Already imported</StatusLabel>}
                                {candidate.disabled && (
                                  <StatusLabel tone="error">Disabled on server</StatusLabel>
                                )}
                                {candidate.isAdmin && <StatusLabel tone="notice">Server admin</StatusLabel>}
                              </Stack>
                              <Stack marginTop={4} render="span">
                                <Text as="span" textRole="hint" tone="secondary">
                                  Initial Loomarr role: {candidate.isAdmin ? "Admin" : "Member"}
                                </Text>
                              </Stack>
                            </label>
                          </Stack>
                        );
                      })}
                    </ul>
                  </Card>
                )}

                <Card flexShrink={0} padding={16}>
                  <div className="grid gap-3 sm:grid-cols-2">
                    <Stack gap="1">
                      <Stack align="center" direction="row" gap="inline" render="p">
                        <Icon decorative glyph={icons.verified} size="compact" tone="success" />
                        <Text as="span" textRole="cardLabel">
                          Passwords stay private
                        </Text>
                      </Stack>
                      <Text as="p" textRole="compact" tone="secondary">
                        Emby/Jellyfin validates the password. After a successful sign-in, Loomarr stores only
                        a non-reversible Argon2id verifier for outage access and refreshes it after later
                        provider sign-ins.
                      </Text>
                    </Stack>
                    <Stack gap="1">
                      <Text as="p" textRole="cardLabel">
                        Sync existing accounts
                      </Text>
                      <Text as="p" textRole="compact" tone="secondary">
                        Refresh imported names and disabled state. Sync never provisions an unselected
                        account.
                      </Text>
                      <Stack direction="row" marginTop={8}>
                        <Action
                          disabled={syncing || importing}
                          onPress={() => void syncExisting()}
                          variant="outline"
                        >
                          {syncing ? "Syncing…" : "Sync existing"}
                        </Action>
                      </Stack>
                    </Stack>
                  </div>
                </Card>
              </Stack>

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
