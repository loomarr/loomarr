import { LoomarrProvider } from "@loomarr/design-system";
import { ClientPlatformProof } from "@loomarr/ui";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

const container = document.getElementById("client-platform-proof");
if (!container) throw new Error("client platform proof root is missing");

createRoot(container).render(
  <StrictMode>
    <LoomarrProvider>
      <ClientPlatformProof />
    </LoomarrProvider>
  </StrictMode>,
);
