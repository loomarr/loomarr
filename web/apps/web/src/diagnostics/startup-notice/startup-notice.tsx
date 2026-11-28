import type { HealthReport } from "@loomarr/api/models/healthReport";
import { useLocation, useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";
import { toast } from "sonner";
import { useCurrentHealth } from "../use-current-health";

// Below the design system's `sm` breakpoint (640 px): the widths the app lays out for a phone.
const PHONE_QUERY = "(max-width: 639.98px)";

const acknowledgedKey = (id: string) => `loomarr.health.ack.${id}`;
const signatureKey = (generationID: string) => `loomarr.health.signature.${generationID}`;

// A build stamped with its commit rather than a release tag reports the full 40-character SHA; the
// notice shows the short form people recognise (#2750). A release tag passes through unchanged.
const displayVersion = (version: string) =>
  /^[0-9a-f]{12,40}$/i.test(version) ? version.slice(0, 7) : version;

const checksNeedingAttention = (count: number) =>
  count === 1 ? "1 check needs attention" : `${count} checks need attention`;

const healthSignature = (health: HealthReport) => {
  const affected = health.checks
    .filter((check) => ["warning", "failed", "stale"].includes(check.status))
    .map((check) => `${check.key}:${check.status}`)
    .sort()
    .join(",");
  return `${health.state}:${affected}`;
};

// One shell-level notice per set of affected checks in a process generation. Dismissing it holds
// until that set changes to one not yet seen or Loomarr restarts: a check that flaps between
// passing and warning would otherwise raise the same notice on every return. Home's status strip
// and Diagnostics still show each recurrence.
const HealthNotice = ({ enabled }: { enabled: boolean }) => {
  const navigate = useNavigate();
  const onHome = useLocation({ select: (location) => location.pathname === "/" });
  const healthQuery = useCurrentHealth({ enabled });
  const health = healthQuery.data?.status === 200 ? healthQuery.data.data : undefined;
  const signature = health ? healthSignature(health) : "";
  const affected =
    health?.checks.filter((check) => ["warning", "failed", "stale"].includes(check.status)).length ?? 0;
  const generationID = health?.generationId;
  const state = health?.state;
  const version = health?.version;

  useEffect(() => {
    if (!generationID || !state || !version || state === "starting") return;
    const lastSignature = localStorage.getItem(signatureKey(generationID));
    if (lastSignature !== signature) localStorage.setItem(signatureKey(generationID), signature);
    const incident = `${generationID}:${signature}`;
    const key = acknowledgedKey(incident);
    if (localStorage.getItem(key)) return;
    // Home's status strip already shows the incident (#2750); the toast would only sit over its
    // cards. It stays unacknowledged, so it still shows once the viewer goes to another page.
    if (onHome && !(state === "healthy" && !lastSignature)) return;

    const acknowledge = () => localStorage.setItem(key, "1");
    // Healthy is the normal state, not an interruption. Remember the initial observation
    // silently; a healthy transition after a visible incident is still useful feedback.
    if (state === "healthy" && !lastSignature) {
      acknowledge();
      return;
    }
    const action = {
      label: "View health",
      onClick: () => {
        acknowledge();
        // Current Health, not the Logs view Diagnostics opens on by default (#2734).
        void navigate({ to: "/settings/system/diagnostics", search: { view: "health" } });
      },
    };
    // A warning stays until dismissed, except on a phone, where it covered the bottom 158 px of
    // every page; there it closes like the healthy notice, and Home's status strip and Diagnostics
    // keep the incident (#1785, the maintainer's call). The width is read once: a toast's duration
    // is fixed when it is raised.
    const phone = typeof window.matchMedia === "function" && window.matchMedia(PHONE_QUERY).matches;
    const duration = state === "healthy" || phone ? 6_000 : Number.POSITIVE_INFINITY;
    const options = {
      description: `${displayVersion(version)}${affected > 0 ? ` · ${checksNeedingAttention(affected)}` : ""}`,
      duration,
      action,
      closeButton: true,
      onDismiss: acknowledge,
      onAutoClose: acknowledge,
    };
    const recovered = lastSignature && !lastSignature.startsWith("healthy:");
    const id =
      state === "unhealthy"
        ? toast.error("Loomarr needs attention", options)
        : state === "degraded"
          ? toast.warning("Loomarr is running with warnings", options)
          : toast.success(recovered ? "Loomarr health recovered" : "Loomarr is healthy", options);
    return () => {
      toast.dismiss?.(id);
    };
  }, [affected, generationID, navigate, onHome, signature, state, version]);

  return null;
};

export { HealthNotice };
