import { createBrowserRouter, Navigate } from "react-router-dom";
import { Layout } from "@/routes/layout";
import { Placeholder } from "@/routes/placeholder";

// Route skeleton (§12 Views). Screens are placeholders in 13.1; the wizard (13.3)
// and product surfaces (13.4) fill them in. Kept flat + declarative so each screen
// is one obvious entry point.
export const router = createBrowserRouter([
  {
    element: <Layout />,
    children: [
      { index: true, element: <Navigate to="/channels" replace /> },
      { path: "channels", element: <Placeholder title="Channels" hint="No channels yet — create your first from an intent." /> },
      { path: "board", element: <Placeholder title="Board" hint="Nothing in flight — approved acquisitions show their journey here." /> },
      { path: "suggest", element: <Placeholder title="Suggest" hint="Describe a channel and Loomarr grounds a lineup against your library." /> },
      { path: "filler", element: <Placeholder title="Filler" hint="No clips yet — drop files in the filler folder or point MeTube at a playlist." /> },
      { path: "users", element: <Placeholder title="Users" hint="Import Emby/Jellyfin accounts to let them sign in." /> },
      { path: "settings", element: <Placeholder title="Settings" hint="Connections, Channels & Playback, AI, Users, Advanced." /> },
      { path: "help", element: <Placeholder title="Help" hint="The embedded docs — searchable, offline." /> },
      { path: "*", element: <Placeholder title="Off the air" hint="That page doesn't exist." /> },
    ],
  },
]);
