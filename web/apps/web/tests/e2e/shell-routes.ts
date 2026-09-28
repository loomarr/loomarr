// The routes that render inside the app shell, with the page title each one exposes. Shared by
// the page-shell contract (page-consistency.spec.ts) and the phone layout gate
// (phone-layout.spec.ts), so a new top-level page joins both by being listed once.
const shellRoutes = [
  { path: "/dashboard", title: "Home" },
  { path: "/guide", title: "Channels" },
  // Requests replaced Queue (#1405). With no requests in this mock, the page shows its empty state
  // under the same header, which is all the shell contract needs.
  { path: "/requests", title: "Requests" },
  { path: "/filler", title: "Filler" },
  { path: "/filler/incoming", title: "Filler" },
  { path: "/filler/sources", title: "Filler" },
  { path: "/filler/settings", title: "Filler" },
  { path: "/people", title: "People" },
  { path: "/settings", title: "Settings" },
  { path: "/settings/connections", title: "Connections" },
  { path: "/settings/ai", title: "AI" },
  { path: "/settings/defaults", title: "Channel defaults" },
  { path: "/settings/access", title: "Access and devices" },
  { path: "/settings/advanced", title: "Advanced settings" },
  { path: "/settings/system/tasks", title: "Tasks" },
  { path: "/settings/system/playback", title: "Playback" },
  { path: "/settings/system/database", title: "Database" },
  { path: "/settings/system/backup", title: "Backup" },
  { path: "/settings/system/storage", title: "Storage" },
  { path: "/settings/system/diagnostics", title: "Diagnostics" },
  { path: "/settings/system/about", title: "About" },
  { path: "/help", title: "Help" },
  { path: "/account", title: "Your account" },
] as const;

export { shellRoutes };
