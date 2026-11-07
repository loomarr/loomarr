import type { ComponentType, ReactNode } from "react";
import type { LibraryCollectionChannel } from "../library-collections/library-collections.type";

/** A title in the collection, or one a search found to add. */
interface LibraryCollectionMember {
  id: string;
  name: string;
  /** Absent when unknown. */
  year?: number;
  /** Drawn by the app; absent draws the name on the placeholder. */
  poster?: ReactNode;
}

/**
 * The app's drag and drop, so the rows can be dragged into a new order. The list holds the rows in
 * order; each row is wrapped by `Row`, and its grip by `Grip`, which is what a pointer or the keyboard
 * picks up. Absent, the rows keep their order.
 */
interface LibraryCollectionSortable {
  List: ComponentType<{
    ids: readonly string[];
    onMove: (from: number, to: number) => void;
    children: ReactNode;
  }>;
  Row: ComponentType<{ id: string; children: ReactNode }>;
  Grip: ComponentType<{ label: string; children: ReactNode }>;
}

interface LibraryCollectionEditorProps {
  /** False for a collection that isn't saved yet: there's nothing to delete. */
  saved: boolean;
  name: string;
  onNameChange: (name: string) => void;
  description: string;
  onDescriptionChange: (description: string) => void;
  /** In the collection's order. */
  members: readonly LibraryCollectionMember[];
  onMove: (from: number, to: number) => void;
  onRemove: (id: string) => void;
  /** Finds the library's titles by name, for "Add a title". */
  onSearch: (query: string) => Promise<readonly LibraryCollectionMember[]>;
  onAdd: (member: LibraryCollectionMember) => void;
  /** The channels that use it: it can't be deleted while any do. */
  usedBy: readonly LibraryCollectionChannel[];
  onBack: () => void;
  onCancel: () => void;
  onSave: () => void;
  onDelete: () => void;
  /** A save or a delete is on its way. */
  busy: boolean;
  /** The last save or delete didn't work. */
  failed?: "save" | "delete";
  sortable?: LibraryCollectionSortable;
}

export type { LibraryCollectionEditorProps, LibraryCollectionMember, LibraryCollectionSortable };
