import * as authApi from "@loomarr/api/endpoints/auth";
import { unwrap } from "@loomarr/api/unwrap";
import { devicePairApproveSchema } from "@loomarr/core/schemas";
import { Action, Field, Stack, Text, Toggle } from "@loomarr/design-system";
import { ConfirmStep } from "@loomarr/ui";
import { useForm } from "@tanstack/react-form";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useAuth } from "@/auth/use-auth";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { useCodeModeDevices, useSetCodeMode } from "@/settings/code-mode";

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

  // A blank code never reaches the server: the schema blocks it, and the button is disabled for
  // it anyway, so there is no message to show.
  const form = useForm({
    defaultValues: { userCode: "" },
    validators: { onSubmit: devicePairApproveSchema },
    onSubmit: ({ value }) => {
      approve.mutate(
        { data: { userCode: value.userCode } },
        {
          onSuccess: (res) => {
            const body = unwrap(res);
            setApproved(body?.deviceName ?? "Device");
            form.reset();
          },
        },
      );
    },
  });

  return (
    <Stack
      borderColor="borderDecorative"
      borderRadius="cardCompact"
      borderWidth={1}
      padding={16}
      render="section"
    >
      <Text as="h2" textRole="cardLabel">
        Add a TV or streaming box
      </Text>
      <Stack marginTop={4}>
        <Text as="p" textRole="compact" tone="secondary">
          Open Loomarr on the device. It will show a short code — type it here to let it sign in as you. The
          device acts with your access and role, and you can remove it at any time.
        </Text>
      </Stack>
      {/* The address to put ON the television. This panel is for an operator already in Settings;
          the device itself cannot direct anyone here, so it names the short route instead. */}
      <Stack marginTop={4}>
        <Text as="p" textRole="compact" tone="secondary">
          Your TV will point at <span className="font-mono">/pair</span> — that page does the same thing and
          is easier to reach from a phone.
        </Text>
      </Stack>
      <Stack
        align="start"
        direction="row"
        gap="2"
        marginTop={12}
        noValidate
        onSubmit={(event) => {
          event.preventDefault();
          void form.handleSubmit();
        }}
        render="form"
        wrap
      >
        <form.Field name="userCode">
          {(field) => (
            <Stack width={176}>
              <Field
                accessibilityLabel="Code shown on the device"
                autoCapitalize="characters"
                autoComplete="off"
                autoCorrect={false}
                onChangeText={field.handleChange}
                placeholder="BCDF-GHJK"
                spellCheck={false}
                style={{ letterSpacing: 1.4, textTransform: "uppercase" }}
                value={field.state.value}
              />
            </Stack>
          )}
        </form.Field>
        <form.Subscribe selector={(state) => state.values.userCode}>
          {(userCode) => (
            <Action disabled={!userCode.trim() || approve.isPending} submit>
              {approve.isPending ? "Checking…" : "Approve device"}
            </Action>
          )}
        </form.Subscribe>
      </Stack>
      {approve.isError ? (
        <Stack marginTop={8}>
          <Text as="p" textRole="compact" tone="danger">
            That code is wrong or has expired. Codes last ten minutes — check the device and try again.
          </Text>
        </Stack>
      ) : null}
      {approved ? (
        <Stack marginTop={8}>
          <Text as="p" textRole="compact">
            <Text as="span" textRole="cardLabel">
              {approved}
            </Text>{" "}
            is approved. It should start playing in a few seconds.
          </Text>
        </Stack>
      ) : null}
    </Stack>
  );
};

// An admin's per-device Code Mode switch (#4486): whether an AI agent paired as this device may run
// scripts against the whole API. Only an admin sees it; the server refuses it to anyone else.
type CodeModeSwitch = { on: boolean; busy: boolean; onChange: (on: boolean) => void };

// Removing signs the device out for good (it has to pair again), so the first press only asks.
const DeviceRow = ({
  device,
  onRevoke,
  busy,
  divided,
  codeMode,
}: {
  device: Device;
  onRevoke: () => void;
  busy: boolean;
  divided: boolean;
  codeMode?: CodeModeSwitch;
}) => {
  const [confirming, setConfirming] = useState(false);
  const name = device.deviceName || "Device";
  return (
    <Stack
      align="center"
      borderBottomColor="borderDecorative"
      borderBottomWidth={divided ? 1 : 0}
      direction="row"
      gap="inline"
      justify="between"
      paddingVertical={12}
      render="li"
      wrap
    >
      <Stack flexShrink={1} minWidth={0}>
        <Text as="p" numberOfLines={1} textRole="cardLabel">
          {name}
        </Text>
        <Text as="p" textRole="hint" tone="secondary">
          Last used {relativeAge(device.lastSeenAt)}
        </Text>
        {codeMode ? (
          <Stack marginTop={8}>
            <Toggle
              checked={codeMode.on}
              description="Lets an AI agent on this device run scripts that use everything you can in Loomarr. Each run is recorded in Diagnostics."
              disabled={codeMode.busy}
              kind="switch"
              label="Code Mode"
              onCheckedChange={codeMode.onChange}
            />
          </Stack>
        ) : null}
      </Stack>
      {confirming ? (
        <ConfirmStep
          confirmLabel="Remove device"
          disabled={busy}
          message={`Remove ${name}? It signs out now and has to be paired again to watch.`}
          onCancel={() => setConfirming(false)}
          onConfirm={() => {
            onRevoke();
            setConfirming(false);
          }}
        />
      ) : (
        <Action disabled={busy} onPress={() => setConfirming(true)} tone="danger" variant="outline">
          {busy ? "Removing…" : "Remove"}
        </Action>
      )}
    </Stack>
  );
};

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
  const { isAdmin } = useAuth();
  const codeModeOn = useCodeModeDevices(isAdmin);
  const setCodeMode = useSetCodeMode();
  // The switch shows only once the server has said which devices have it on, so it never
  // flashes "off" for a device that is on.
  const codeModeFor = (id: string): CodeModeSwitch | undefined =>
    isAdmin && codeModeOn.data
      ? {
          on: codeModeOn.data.has(id),
          busy: setCodeMode.isPending && setCodeMode.variables?.id === id,
          onChange: (on) => setCodeMode.mutate({ id, on }),
        }
      : undefined;

  return (
    <>
      <ApproveDevice />
      <Stack borderColor="$borderDecorative" borderRadius={8} borderWidth={1} padding={16} render="section">
        <Text as="h2" textRole="cardLabel">
          Your devices
        </Text>
        <Stack marginTop={4}>
          <Text as="p" textRole="compact" tone="secondary">
            Devices signed in as you. Removing one signs it out immediately; your other devices keep working.
          </Text>
        </Stack>
        {query.isError ? <ErrorState error={query.error} /> : null}
        {!query.isError && devices.length === 0 ? (
          <EmptyState description="Approve a code above to add your first TV." title="No devices yet" />
        ) : null}
        {devices.length > 0 ? (
          <Stack marginTop={8} render="ul">
            {devices.map((device, index) => (
              <DeviceRow
                busy={revoke.isPending && revoke.variables?.id === device.id}
                codeMode={codeModeFor(device.id)}
                device={device}
                divided={index < devices.length - 1}
                key={device.id}
                onRevoke={() => revoke.mutate({ id: device.id })}
              />
            ))}
          </Stack>
        ) : null}
      </Stack>
    </>
  );
};

export { PairedDevices };
