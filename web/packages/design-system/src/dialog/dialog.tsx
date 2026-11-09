import { View } from "@tamagui/core";
import { cloneElement, createContext, type ReactElement, useContext, useId, useState } from "react";
import { Modal, Pressable, ScrollView } from "react-native";

import { Icon } from "../icon";
import { icons } from "../icons";
import { ModalLayer } from "../modal-layer";
import { useReducedMotionPreference } from "../motion/use-reduced-motion";
import { Text } from "../primitives";
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
  open: boolean;
  setOpen: (open: boolean) => void;
  titleId: string;
};

const DialogContext = createContext<DialogState | undefined>(undefined);
const LayoutContext = createContext<NonNullable<DialogContentProps["layout"]>>("padded");

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

const Dialog = ({ children, defaultOpen = false, onOpenChange, open }: DialogProps) => {
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
        open: open ?? uncontrolledOpen,
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
  layout = "padded",
  size = "default",
}: DialogContentProps) => {
  const { descriptionId, open, setOpen, titleId } = useDialog();
  const reducedMotion = useReducedMotionPreference();
  return (
    <Modal
      animationType={reducedMotion === false ? "fade" : "none"}
      navigationBarTranslucent
      onRequestClose={() => setOpen(false)}
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
            onPress={() => setOpen(false)}
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
              maxWidth={maxWidths[size]}
              overflow="hidden"
              width="100%"
            >
              <ScrollView contentContainerStyle={{ gap: 16, padding: layout === "sectioned" ? 0 : 24 }}>
                <LayoutContext.Provider value={layout}>{children}</LayoutContext.Provider>
              </ScrollView>
              <Pressable
                accessibilityLabel="Close"
                accessibilityRole="button"
                onPress={() => setOpen(false)}
                style={{ position: "absolute", right: 16, top: 16 }}
              >
                <Icon decorative glyph={icons.close} size="compact" tone="secondary" />
              </Pressable>
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

const DialogFooter = ({ children, spread = false }: DialogFooterProps) => {
  const sectioned = useContext(LayoutContext) === "sectioned";
  return (
    <View
      flexDirection="row"
      flexWrap={spread ? "wrap" : "nowrap"}
      gap={8}
      justifyContent={spread ? "space-between" : "flex-end"}
      {...(sectioned
        ? { borderTopColor: "surfaceElevated", borderTopWidth: 1, paddingHorizontal: 24, paddingVertical: 16 }
        : {})}
    >
      {children}
    </View>
  );
};

// A padded title keeps clear of the corner close; a sectioned header already does.
const DialogTitle = ({ children }: DialogSectionProps) => {
  const { titleId } = useDialog();
  const padded = useContext(LayoutContext) === "padded";
  return (
    <Text accessibilityRole="header" id={titleId} paddingRight={padded ? 32 : 0} textRole="title">
      {children}
    </Text>
  );
};

const DialogDescription = ({ children }: DialogSectionProps) => {
  const { descriptionId } = useDialog();
  return (
    <Text id={descriptionId} textRole="body" tone="secondary">
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
