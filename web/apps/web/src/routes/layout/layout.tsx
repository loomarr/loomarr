import { useLoomarrEvents } from "@loomarr/core";
import { useState } from "react";
import { Outlet } from "react-router-dom";
import { AppShell } from "@/components/loomarr";

// The app layout: the AppShell frame around the routed content, plus the SSE
// invalidation stream for the app's lifetime (core.useLoomarrEvents keeps every
// surface live — main doc §12). The ⌘K command palette is wired in 13.4; for now
// the shell's search button toggles a stub.
const Layout = () => {
  const [, setCommandOpen] = useState(false);
  useLoomarrEvents();

  return (
    <AppShell onOpenCommand={() => setCommandOpen(true)}>
      <Outlet />
    </AppShell>
  );
};

export { Layout };
