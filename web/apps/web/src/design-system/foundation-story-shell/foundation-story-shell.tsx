import { LoomarrProvider, semanticColors } from "@loomarr/design-system";
import type { PropsWithChildren } from "react";

const FoundationsStoryShell = ({ children }: PropsWithChildren) => (
  <LoomarrProvider theme="dark">
    <div
      style={{
        background: semanticColors.surface.canvas,
        boxSizing: "border-box",
        color: semanticColors.content.primary,
        fontFamily: "'Geist Variable', Geist, sans-serif",
        minHeight: "100vh",
        padding: 32,
        width: "100%",
      }}
    >
      {children}
    </div>
  </LoomarrProvider>
);

export { FoundationsStoryShell };
