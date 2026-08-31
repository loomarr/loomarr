import { Drawer as DrawerPrimitive } from "@base-ui/react/drawer";
import { X } from "lucide-react";
import { cn } from "@/lib/utils";
import type {
  SheetContentProps,
  SheetDescriptionProps,
  SheetSectionProps,
  SheetTitleProps,
} from "./sheet.type";

const Sheet = DrawerPrimitive.Root;
const SheetTrigger = DrawerPrimitive.Trigger;
const SheetClose = DrawerPrimitive.Close;

const SheetContent = ({ className, children, ...props }: SheetContentProps) => (
  <DrawerPrimitive.Portal>
    <DrawerPrimitive.Backdrop className="fixed inset-0 z-50 bg-black/60 transition-opacity data-ending-style:opacity-0 data-starting-style:opacity-0" />
    <DrawerPrimitive.Viewport className="fixed inset-0 z-50 flex items-stretch justify-end">
      <DrawerPrimitive.Popup
        className={cn(
          "h-full w-full overflow-y-auto border-static-700 border-l bg-card text-card-foreground shadow-xl outline-none transition-transform duration-300 ease-out data-ending-style:translate-x-full data-starting-style:translate-x-full sm:max-w-xl",
          className,
        )}
        {...props}
      >
        <DrawerPrimitive.Content className="min-h-full">{children}</DrawerPrimitive.Content>
        <DrawerPrimitive.Close
          className="absolute top-4 right-4 rounded-sm text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          aria-label="Close"
        >
          <X className="size-5" aria-hidden />
        </DrawerPrimitive.Close>
      </DrawerPrimitive.Popup>
    </DrawerPrimitive.Viewport>
  </DrawerPrimitive.Portal>
);

const SheetHeader = ({ className, ...props }: SheetSectionProps) => (
  <div className={cn("flex flex-col gap-1.5 border-static-800 border-b p-6 pr-14", className)} {...props} />
);

const SheetTitle = ({ className, ...props }: SheetTitleProps) => (
  <DrawerPrimitive.Title className={cn("font-semibold text-xl", className)} {...props} />
);

const SheetDescription = ({ className, ...props }: SheetDescriptionProps) => (
  <DrawerPrimitive.Description className={cn("text-muted-foreground text-sm", className)} {...props} />
);

export { Sheet, SheetClose, SheetContent, SheetDescription, SheetHeader, SheetTitle, SheetTrigger };
