import * as libraryApi from "@loomarr/api/endpoints/library";
import type { LibraryItemDTO } from "@loomarr/api/models/libraryItemDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { LibraryCollectionEditor, type LibraryCollectionMember, StatePanel } from "@loomarr/ui";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useCallback, useEffect, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { ServedImage } from "@/components/loomarr/served-image";
import { SORTABLE } from "../collection-sortable";

// THE COLLECTION EDITOR (#2151 decision 28): a household collection's name, description and titles
// in order, saved together. Without an id it makes a new one. Delete waits while a channel uses it;
// the server refuses it too.

const THUMB_SIZES = "36px";
const SEARCH_LIMIT = 8;

const memberOf = (item: LibraryItemDTO): LibraryCollectionMember => ({
  id: item.id,
  name: item.name,
  year: item.year,
  poster: item.poster ? <ServedImage alt="" image={item.poster} sizes={THUMB_SIZES} /> : undefined,
});

const move = <T,>(list: readonly T[], from: number, to: number) => {
  const next = [...list];
  const [it] = next.splice(from, 1);
  if (it !== undefined) next.splice(to, 0, it);
  return next;
};

interface Draft {
  name: string;
  description: string;
  members: LibraryCollectionMember[];
}

const CollectionEditorPage = ({ id }: { id?: string }) => {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const saved = libraryApi.useGetCollection(id ?? "", { query: { enabled: id !== undefined } });
  const current = unwrap(saved.data);
  const [draft, setDraft] = useState<Draft | null>(
    id === undefined ? { name: "", description: "", members: [] } : null,
  );
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState<"save" | "delete">();

  // The saved collection fills the draft once it arrives.
  useEffect(() => {
    if (current && draft === null) {
      setDraft({
        name: current.name,
        description: current.description,
        members: current.titles.map(memberOf),
      });
    }
  }, [current, draft]);

  const onSearch = useCallback(async (query: string) => {
    const body = unwrap(await libraryApi.listLibraryItems({ search: query, limit: SEARCH_LIMIT }));
    if (!body) throw new Error("Couldn't search the library");
    return body.items.map(memberOf);
  }, []);

  const back = () => void navigate({ to: "/library/collections" });
  const done = async () => {
    await queryClient.invalidateQueries({ queryKey: libraryApi.getListCollectionsQueryKey() });
    if (id) await queryClient.invalidateQueries({ queryKey: libraryApi.getGetCollectionQueryKey(id) });
    back();
  };

  const save = async () => {
    if (!draft) return;
    setBusy(true);
    setFailed(undefined);
    const body = {
      name: draft.name.trim(),
      description: draft.description.trim(),
      titles: draft.members.map((m) => m.id),
    };
    try {
      await (id ? libraryApi.updateCollection(id, body) : libraryApi.createCollection(body));
      await done();
    } catch {
      setFailed("save");
      setBusy(false);
    }
  };

  const remove = async () => {
    if (!id) return;
    setBusy(true);
    setFailed(undefined);
    try {
      await libraryApi.deleteCollection(id);
      await done();
    } catch {
      setFailed("delete");
      setBusy(false);
    }
  };

  if (saved.error && !current) return <ErrorState error={saved.error} onRetry={() => void saved.refetch()} />;
  if (!draft) return <StatePanel kind="loading" title="Loading this collection" />;

  return (
    <LibraryCollectionEditor
      busy={busy}
      description={draft.description}
      failed={failed}
      members={draft.members}
      name={draft.name}
      onAdd={(m) => setDraft({ ...draft, members: [...draft.members, m] })}
      onBack={back}
      onCancel={back}
      onDelete={() => void remove()}
      onDescriptionChange={(description) => setDraft({ ...draft, description })}
      onMove={(from, to) => setDraft({ ...draft, members: move(draft.members, from, to) })}
      onNameChange={(name) => setDraft({ ...draft, name })}
      onRemove={(memberId) => setDraft({ ...draft, members: draft.members.filter((m) => m.id !== memberId) })}
      onSave={() => void save()}
      onSearch={onSearch}
      saved={id !== undefined}
      sortable={SORTABLE}
      usedBy={current?.usedBy ?? []}
    />
  );
};

export { CollectionEditorPage };
