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
          "border-static-700 bg-card text-card-foreground h-full w-full overflow-y-auto border-l shadow-xl transition-transform duration-300 ease-out outline-none data-ending-style:translate-x-full data-starting-style:translate-x-full sm:max-w-xl",
          className,
        )}
        {...props}
      >
        <DrawerPrimitive.Content className="min-h-full">{children}</DrawerPrimitive.Content>
        <DrawerPrimitive.Close
          className="text-muted-foreground hover:text-foreground focus-visible:ring-ring absolute top-4 right-4 rounded-sm transition-colors focus-visible:ring-2 focus-visible:outline-none"
          aria-label="Close"
        >
          <X className="size-5" aria-hidden />
        </DrawerPrimitive.Close>
      </DrawerPrimitive.Popup>
    </DrawerPrimitive.Viewport>
  </DrawerPrimitive.Portal>
);

const SheetHeader = ({ className, ...props }: SheetSectionProps) => (
  <div className={cn("border-static-800 flex flex-col gap-1.5 border-b p-6 pr-14", className)} {...props} />
);

const SheetTitle = ({ className, ...props }: SheetTitleProps) => (
  <DrawerPrimitive.Title className={cn("text-xl font-semibold", className)} {...props} />
);

const SheetDescription = ({ className, ...props }: SheetDescriptionProps) => (
  <DrawerPrimitive.Description className={cn("text-muted-foreground text-sm", className)} {...props} />
);

export { Sheet, SheetClose, SheetContent, SheetDescription, SheetHeader, SheetTitle, SheetTrigger };
