import { View } from "@tamagui/core";
import { cloneElement, createContext, type ReactElement, useContext, useId, useState } from "react";
import { Modal, Pressable, ScrollView } from "react-native";

import { Icon } from "../icon";
import { icons } from "../icons";
import { ModalLayer } from "../modal-layer";
import { useReducedMotionPreference } from "../motion/use-reduced-motion";
import { Text } from "../text";
import { semanticRadius } from "../tokens";
import { ViewportStage } from "../viewport";
import type {
  DialogActionProps,
  DialogContentProps,
  DialogFooterProps,
  DialogProps,
  DialogSectionProps,
} from "./dialog.type";

// The native adapter, on React Native's Modal: the platform moves focus into the modal's own window
// and back out, and Back (Android) or Menu (tvOS) closes it through `onRequestClose`, which is
// Escape's place here. The panel draws the web dialog's values: the raised card, the decorative
// hairline, the 60% scrim, the corner close.

type DialogState = {
  descriptionId: string;
  dismissible: boolean;
  open: boolean;
  requestClose: () => void;
  setOpen: (open: boolean) => void;
  titleId: string;
};

const DialogContext = createContext<DialogState | undefined>(undefined);
const LayoutContext = createContext<NonNullable<DialogContentProps["layout"]>>("padded");
const DensityContext = createContext<NonNullable<DialogContentProps["density"]>>("touch");

// A television's panel is wider than any web size, and it has no close key: Back closes it.
const TV_WIDTH = 880;

const maxWidths: Record<NonNullable<DialogContentProps["size"]>, number> = {
  default: 448,
  form: 672,
  reading: 768,
};

const useDialog = () => {
  const state = useContext(DialogContext);
  if (state === undefined) throw new Error("Dialog parts render inside a Dialog.");
  return state;
};

const Dialog = ({ children, defaultOpen = false, dismissible = true, onOpenChange, open }: DialogProps) => {
  const id = useId();
  const [uncontrolledOpen, setUncontrolledOpen] = useState(defaultOpen);
  const setOpen = (next: boolean) => {
    if (open === undefined) setUncontrolledOpen(next);
    onOpenChange?.(next);
  };
  return (
    <DialogContext.Provider
      value={{
        descriptionId: `${id}-description`,
        dismissible,
        open: open ?? uncontrolledOpen,
        // Back, the scrim and the corner close ask to close; a dialog that must be answered refuses
        // them. Its own DialogClose and `open` still close it.
        requestClose: () => {
          if (dismissible) setOpen(false);
        },
        setOpen,
        titleId: `${id}-title`,
      }}
    >
      {children}
    </DialogContext.Provider>
  );
};

type PressableElement = ReactElement<{ onPress?: () => void }>;

const DialogTrigger = ({ children }: DialogActionProps) => {
  const { setOpen } = useDialog();
  return cloneElement(children as PressableElement, { onPress: () => setOpen(true) });
};

const DialogClose = ({ children }: DialogActionProps) => {
  const { setOpen } = useDialog();
  return cloneElement(children as PressableElement, { onPress: () => setOpen(false) });
};

const DialogContent = ({
  accessibilityLabel,
  children,
  density = "touch",
  layout = "padded",
  size = "default",
}: DialogContentProps) => {
  const { descriptionId, dismissible, open, requestClose, titleId } = useDialog();
  const reducedMotion = useReducedMotionPreference();
  const tv = density === "tv";
  return (
    <Modal
      animationType={reducedMotion === false ? "fade" : "none"}
      navigationBarTranslucent
      onRequestClose={requestClose}
      presentationStyle="overFullScreen"
      statusBarTranslucent
      transparent
      visible={open}
    >
      {/* A native modal is a root of its own, outside the app's viewport stage. */}
      <ViewportStage>
        <View alignItems="center" flex={1} justifyContent="center" padding={16}>
          <Pressable
            accessible={false}
            importantForAccessibility="no-hide-descendants"
            onPress={requestClose}
            style={{
              backgroundColor: "rgba(0, 0, 0, 0.6)",
              bottom: 0,
              left: 0,
              position: "absolute",
              right: 0,
              top: 0,
            }}
          />
          <ModalLayer visible={open}>
            <View
              accessibilityLabel={accessibilityLabel}
              accessibilityViewIsModal
              aria-describedby={descriptionId}
              aria-labelledby={accessibilityLabel ? undefined : titleId}
              backgroundColor="surfaceRaised"
              borderColor="borderDecorative"
              borderRadius={semanticRadius.cardCompact}
              borderWidth={1}
              maxHeight="100%"
              maxWidth={tv ? TV_WIDTH : maxWidths[size]}
              overflow="hidden"
              width="100%"
            >
              <ScrollView
                contentContainerStyle={{ gap: 16, padding: layout === "sectioned" ? 0 : tv ? 32 : 24 }}
              >
                <DensityContext.Provider value={density}>
                  <LayoutContext.Provider value={layout}>{children}</LayoutContext.Provider>
                </DensityContext.Provider>
              </ScrollView>
              {dismissible && !tv ? (
                <Pressable
                  accessibilityLabel="Close"
                  accessibilityRole="button"
                  onPress={requestClose}
                  style={{ position: "absolute", right: 16, top: 16 }}
                >
                  <Icon decorative glyph={icons.close} size="compact" tone="secondary" />
                </Pressable>
              ) : null}
            </View>
          </ModalLayer>
        </View>
      </ViewportStage>
    </Modal>
  );
};

const DialogHeader = ({ children }: DialogSectionProps) => {
  const sectioned = useContext(LayoutContext) === "sectioned";
  return (
    <View
      gap={6}
      {...(sectioned
        ? {
            borderBottomColor: "surfaceElevated",
            borderBottomWidth: 1,
            paddingBottom: 16,
            paddingLeft: 24,
            paddingRight: 48,
            paddingTop: 24,
          }
        : {})}
    >
      {children}
    </View>
  );
};

const DialogFooter = ({ children, spread = false, stacked = false }: DialogFooterProps) => {
  const sectioned = useContext(LayoutContext) === "sectioned";
  return (
    <View
      {...(stacked
        ? { alignItems: "stretch", flexDirection: "column" }
        : {
            flexDirection: "row",
            flexWrap: spread ? "wrap" : "nowrap",
            justifyContent: spread ? "space-between" : "flex-end",
          })}
      gap={8}
      {...(sectioned
        ? { borderTopColor: "surfaceElevated", borderTopWidth: 1, paddingHorizontal: 24, paddingVertical: 16 }
        : {})}
    >
      {children}
    </View>
  );
};

// A padded title keeps clear of the corner close, when one is drawn; a sectioned header already does.
const DialogTitle = ({ children }: DialogSectionProps) => {
  const { dismissible, titleId } = useDialog();
  const density = useContext(DensityContext);
  const clearOfClose = useContext(LayoutContext) === "padded" && dismissible && density !== "tv";
  return (
    <Text
      accessibilityRole="header"
      density={density}
      id={titleId}
      paddingRight={clearOfClose ? 32 : 0}
      textRole="title"
    >
      {children}
    </Text>
  );
};

const DialogDescription = ({ children }: DialogSectionProps) => {
  const { descriptionId } = useDialog();
  return (
    <Text density={useContext(DensityContext)} id={descriptionId} textRole="body" tone="secondary">
      {children}
    </Text>
  );
};

export {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
};
