import * as diagnosticsApi from "@loomarr/api/endpoints/diagnostics";
import type { HealthReport } from "@loomarr/api/models/healthReport";
import { useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";
import { toast } from "sonner";

// Below Tailwind's `sm` breakpoint (640 px): the widths the app lays out for a phone.
const PHONE_QUERY = "(max-width: 639.98px)";

const acknowledgedKey = (id: string) => `loomarr.health.ack.${id}`;
const signatureKey = (generationID: string) => `loomarr.health.signature.${generationID}`;
const sequenceKey = (generationID: string) => `loomarr.health.sequence.${generationID}`;

const healthSignature = (health: HealthReport) => {
  const affected = health.checks
    .filter((check) => ["warning", "failed", "stale"].includes(check.status))
    .map((check) => `${check.key}:${check.status}`)
    .sort()
    .join(",");
  return `${health.state}:${affected}`;
};

// One shell-level notice per material health transition. Its generation-local sequence stays
// stable across ordinary refresh timestamps, so a persistent incident never flickers or repeats.
const HealthNotice = ({ enabled }: { enabled: boolean }) => {
  const navigate = useNavigate();
  const healthQuery = diagnosticsApi.useGetCurrentHealth({
    query: { enabled, retry: false, refetchInterval: 10_000 },
  });
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
    let sequence = Number.parseInt(localStorage.getItem(sequenceKey(generationID)) ?? "0", 10);
    if (lastSignature !== signature) {
      sequence += 1;
      localStorage.setItem(signatureKey(generationID), signature);
      localStorage.setItem(sequenceKey(generationID), String(sequence));
    }
    const incident = `${generationID}:${sequence}:${signature}`;
    const key = acknowledgedKey(incident);
    if (localStorage.getItem(key)) return;

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
        void navigate({ to: "/settings/system/diagnostics" });
      },
    };
    // A warning stays until dismissed, except on a phone, where it covered the bottom 158 px of
    // every page; there it closes like the healthy notice, and Home's status strip and Diagnostics
    // keep the incident (#1785, the maintainer's call). The width is read once: a toast's duration
    // is fixed when it is raised.
    const phone = typeof window.matchMedia === "function" && window.matchMedia(PHONE_QUERY).matches;
    const duration = state === "healthy" || phone ? 6_000 : Number.POSITIVE_INFINITY;
    const options = {
      description: `${version}${affected > 0 ? ` · ${affected} check${affected === 1 ? "" : "s"} need attention` : ""}`,
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
  }, [affected, generationID, navigate, signature, state, version]);

  return null;
};

export { HealthNotice };
