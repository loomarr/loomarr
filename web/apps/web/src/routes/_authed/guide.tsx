import { createFileRoute } from "@tanstack/react-router";
import { GuidePage } from "@/channels";

// The cross-channel time grid (§12). Readable by any authenticated user — the guide is
// viewer-facing, the same posture as the channel list it complements, and GET /v1/guide is
// likewise not admin-gated.
//
// ⚠ This is the grid ONLY. The v2 IA rename (/channels → /guide, /users → /people, and the
// two role-specific navs) is deliberately a separate change: bundling them would put the
// rename's mechanical churn and the grid's new pixels in one visual-baseline diff, where
// neither can be reviewed independently.
const GuideScreen = () => <GuidePage />;

const Route = createFileRoute("/_authed/guide")({ component: GuideScreen });

export { Route };
