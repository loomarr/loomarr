import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { RouterProvider } from "react-router-dom";
import { Toaster } from "sonner";
import { router } from "@/router";
// Self-hosted Geist (§2.2) — bundled by Vite, no CDN, deterministic visual tests.
import "@fontsource-variable/geist";
import "@fontsource-variable/geist-mono";
import "@/styles.css";

// Server state is TanStack Query, SSE-invalidated (no global store, §4.3). Retries
// are conservative — most failures here are 401/403/501 that a retry won't fix.
const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, refetchOnWindowFocus: false, staleTime: 15_000 },
    mutations: { retry: 0 },
  },
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
      <Toaster theme="dark" position="bottom-right" richColors />
    </QueryClientProvider>
  </StrictMode>,
);
