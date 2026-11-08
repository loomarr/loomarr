import {
  DndContext,
  type DragEndEvent,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import type { LibraryCollectionSortable } from "@loomarr/ui";
import { createContext, type ReactNode, useContext } from "react";

// The collection editor's drag and drop (#2151 decision 28): a row moves by its grip, with a pointer
// or with the keyboard (Space picks it up, the arrows move it, Space drops it). The editor draws the
// rows; this only wraps them.

type Handle = Pick<ReturnType<typeof useSortable>, "attributes" | "listeners" | "setActivatorNodeRef">;
const HandleContext = createContext<Handle | null>(null);

const List: LibraryCollectionSortable["List"] = ({ ids, onMove, children }) => {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
  // The order changes once, on the drop.
  const onDragEnd = ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return;
    const from = ids.indexOf(String(active.id));
    const to = ids.indexOf(String(over.id));
    if (from !== -1 && to !== -1) onMove(from, to);
  };
  return (
    <DndContext onDragEnd={onDragEnd} sensors={sensors}>
      <SortableContext items={[...ids]} strategy={verticalListSortingStrategy}>
        {children}
      </SortableContext>
    </DndContext>
  );
};

const Row: LibraryCollectionSortable["Row"] = ({ id, children }) => {
  const { attributes, listeners, setActivatorNodeRef, setNodeRef, transform, transition, isDragging } =
    useSortable({ id });
  return (
    <div
      ref={setNodeRef}
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
        position: "relative",
        zIndex: isDragging ? 1 : undefined,
      }}
    >
      <HandleContext.Provider value={{ attributes, listeners, setActivatorNodeRef }}>
        {children}
      </HandleContext.Provider>
    </div>
  );
};

const Grip: LibraryCollectionSortable["Grip"] = ({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) => {
  const handle = useContext(HandleContext);
  return (
    <button
      {...handle?.attributes}
      {...handle?.listeners}
      aria-label={label}
      ref={handle?.setActivatorNodeRef}
      style={{ background: "none", border: 0, cursor: "grab", padding: 4, touchAction: "none" }}
      type="button"
    >
      {children}
    </button>
  );
};

const SORTABLE: LibraryCollectionSortable = { List, Row, Grip };

export { SORTABLE };
