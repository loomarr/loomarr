import type { Drawer as DrawerPrimitive } from "@base-ui/react/drawer";
import type * as React from "react";

type SheetContentProps = DrawerPrimitive.Popup.Props;
type SheetSectionProps = React.HTMLAttributes<HTMLDivElement>;
type SheetTitleProps = DrawerPrimitive.Title.Props;
type SheetDescriptionProps = DrawerPrimitive.Description.Props;

export type { SheetContentProps, SheetDescriptionProps, SheetSectionProps, SheetTitleProps };
