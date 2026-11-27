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
import { Action } from "@loomarr/design-system";
import type { LibraryCollectionSortable } from "@loomarr/ui";
import { type ComponentRef, createContext, type Ref, useContext } from "react";
import type { ViewStyle } from "react-native";

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

// The grab cursor and `touch-action: none` (a touch drag must move the row, not scroll the page) are
// react-native-web style properties that React Native's ViewStyle does not list.
const GRIP_STYLE = { cursor: "grab", touchAction: "none" } as unknown as ViewStyle;

// The grip is the DS's drag handle (a compact ghost icon Action), the same one the lineup and rules
// editors use, so the `children` the editor passes (its native fallback's glyph) are not drawn here.
const Grip: LibraryCollectionSortable["Grip"] = ({ label }) => {
  const handle = useContext(HandleContext);
  // The Action is already a focusable button; dnd-kit's own role and tabIndex would only repeat it.
  const { role: _role, tabIndex: _tabIndex, ...attributes } = handle?.attributes ?? {};
  return (
    <Action
      {...attributes}
      {...handle?.listeners}
      compact
      icon="dragHandle"
      layout="icon"
      // dnd-kit hands out a DOM ref; Action's is typed as React Native's element, which on the web
      // is that same DOM node.
      ref={handle?.setActivatorNodeRef as unknown as Ref<ComponentRef<typeof Action>>}
      style={GRIP_STYLE}
      variant="ghost"
    >
      {label}
    </Action>
  );
};

const SORTABLE: LibraryCollectionSortable = { List, Row, Grip };

export { SORTABLE };
