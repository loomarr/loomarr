import * as authApi from "@loomarr/api/endpoints/auth";
import { unwrap } from "@loomarr/api/unwrap";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

// The two halves of device pairing, in the order an operator meets them: approve the code a TV is
// showing, then manage what is already paired (§11, Shield P1).
//
// This lives here rather than in a settings BLOCK because it is not a setting — nothing in the
// registry declares it. It is a live list plus one action, so it belongs in the page footer beside
// the secrets panel, which is the same kind of thing.

// A paired device, as its owner sees it. `id` is the token HASH, never the token — it is a stable
// revoke handle that cannot be replayed as a credential.
type Device = {
  id: string;
  deviceName: string;
  createdAt: string;
  lastSeenAt: string;
};

// A device's clock is not ours and its last-seen instant is only meaningful relative to now, so this
// renders a coarse relative age rather than a timestamp nobody can act on.
const relativeAge = (iso: string): string => {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return "unknown";
  const minutes = Math.floor((Date.now() - then) / 60_000);
  if (minutes < 2) return "just now";
  if (minutes < 60) return `${minutes} minutes ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} ${hours === 1 ? "hour" : "hours"} ago`;
  const days = Math.floor(hours / 24);
  return `${days} ${days === 1 ? "day" : "days"} ago`;
};

const ApproveDevice = () => {
  const [code, setCode] = useState("");
  const [approved, setApproved] = useState<string | null>(null);
  const queryClient = useQueryClient();
  const approve = authApi.useDevicePairApprove({
    mutation: {
      onSuccess: () => {
        // The newly approved device only appears in the list once it polls and redeems, but
        // refetching is still right: the operator's next question is "did it work", and a stale
        // list is the least helpful possible answer.
        void queryClient.invalidateQueries({ queryKey: authApi.getDeviceListQueryKey() });
      },
    },
  });

  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    if (!code.trim()) return;
    approve.mutate(
      { data: { userCode: code } },
      {
        onSuccess: (res) => {
          const body = unwrap(res);
          setApproved(body?.deviceName ?? "Device");
          setCode("");
        },
      },
    );
  };

  return (
    <section className="rounded-lg border border-border p-4">
      <h2 className="font-medium text-sm">Add a TV or streaming box</h2>
      <p className="mt-1 text-muted-foreground text-sm leading-relaxed">
        Open Loomarr on the device. It will show a short code — type it here to let it sign in as you. The
        device gets your access, not admin access, and you can remove it at any time.
      </p>
      {/* The address to put ON the television. This panel is for an operator already in Settings;
          the device itself cannot direct anyone here, so it names the short route instead. */}
      <p className="mt-1 text-muted-foreground text-sm leading-relaxed">
        Your TV will point at <span className="font-mono">/pair</span> — that page does the same thing and is
        easier to reach from a phone.
      </p>
      <form className="mt-3 flex flex-wrap items-start gap-2" onSubmit={submit}>
        <Input
          aria-label="Code shown on the device"
          autoComplete="off"
          className="w-44 font-mono uppercase tracking-widest"
          onChange={(event) => setCode(event.target.value)}
          placeholder="BCDF-GHJK"
          spellCheck={false}
          value={code}
        />
        <Button disabled={!code.trim() || approve.isPending} type="submit">
          {approve.isPending ? "Checking…" : "Approve device"}
        </Button>
      </form>
      {approve.isError ? (
        <p className="mt-2 text-destructive text-sm">
          That code is wrong or has expired. Codes last ten minutes — check the device and try again.
        </p>
      ) : null}
      {approved ? (
        <p className="mt-2 text-sm">
          <span className="font-medium">{approved}</span> is approved. It should start playing in a few
          seconds.
        </p>
      ) : null}
    </section>
  );
};

const DeviceRow = ({ device, onRevoke, busy }: { device: Device; onRevoke: () => void; busy: boolean }) => (
  <li className="flex flex-wrap items-center justify-between gap-2 border-border border-b py-3 last:border-b-0">
    <div className="min-w-0">
      <p className="truncate font-medium text-sm">{device.deviceName || "Device"}</p>
      <p className="text-muted-foreground text-xs">Last used {relativeAge(device.lastSeenAt)}</p>
    </div>
    <Button disabled={busy} onClick={onRevoke} size="sm" variant="outline">
      {busy ? "Removing…" : "Remove"}
    </Button>
  </li>
);

const PairedDevices = () => {
  const queryClient = useQueryClient();
  const query = useQuery(authApi.getDeviceListQueryOptions());
  const revoke = authApi.useDeviceRevoke({
    mutation: {
      onSuccess: () => {
        void queryClient.invalidateQueries({ queryKey: authApi.getDeviceListQueryKey() });
      },
    },
  });

  const body = query.data ? unwrap(query.data) : undefined;
  const devices: Device[] = body?.devices ?? [];

  return (
    <>
      <ApproveDevice />
      <section className="rounded-lg border border-border p-4">
        <h2 className="font-medium text-sm">Your devices</h2>
        <p className="mt-1 text-muted-foreground text-sm leading-relaxed">
          Devices signed in as you. Removing one signs it out immediately; your other devices keep working.
        </p>
        {query.isError ? <ErrorState error={query.error} /> : null}
        {!query.isError && devices.length === 0 ? (
          <EmptyState description="Approve a code above to add your first TV." title="No devices yet" />
        ) : null}
        {devices.length > 0 ? (
          <ul className="mt-2">
            {devices.map((device) => (
              <DeviceRow
                busy={revoke.isPending && revoke.variables?.id === device.id}
                device={device}
                key={device.id}
                onRevoke={() => revoke.mutate({ id: device.id })}
              />
            ))}
          </ul>
        ) : null}
      </section>
    </>
  );
};

export { PairedDevices };
