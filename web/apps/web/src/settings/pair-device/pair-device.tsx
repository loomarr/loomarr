import * as authApi from "@loomarr/api/endpoints/auth";
import { ApiError } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { devicePairApproveSchema } from "@loomarr/core/schemas";
import { Action, Field, Stack, Text } from "@loomarr/design-system";
import { useForm } from "@tanstack/react-form";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useAuth } from "@/auth/use-auth";
import { useOAuthRequest } from "@/settings/oauth-request";

// The pairing screen a TV sends someone to (§11, Shield P1) — the address the device puts on
// screen, so it must be SHORT and memorable: `<your-server>/pair`.
//
// A single-task page, sized for its actual context: read a code off a television across the room,
// type it one-thumbed on a phone. That is closer to an OTP field than a settings input, which is
// why the control is oversized, monospaced, and widely tracked rather than matching form density
// elsewhere in the app.

type PairDeviceProps = {
  // The code from `?code=`, when the visitor arrived from a link or a scan.
  initialCode?: string;
  // Set when an app's sign-in (OAuth, #4485) sent the visitor here: the server path that finishes
  // it, already validated by the route. Approving then continues there instead of confirming.
  next?: string;
};

// Signed-out is an EXPECTED arrival state here, not an error: the person holding the remote is
// often not yet signed in on the phone they are typing on. So the page says what to do and hands
// them a link back to this exact URL — code (and an app sign-in's way back) included — rather than
// a bare "unauthorized".
const SignInFirst = ({ code, next }: { code: string; next?: string }) => {
  const params = new URLSearchParams();
  if (code) params.set("code", code);
  if (next) params.set("next", next);
  const query = params.toString();
  const here = `/pair${query ? `?${query}` : ""}`;
  return (
    <Stack marginHorizontal="auto" maxWidth={512} paddingHorizontal={24} paddingVertical={64} width="100%">
      <Text as="h1" strong textAlign="center" textRole="display">
        Add a device
      </Text>
      <Stack marginTop={12}>
        <Text as="p" textAlign="center" textRole="reading" tone="secondary">
          Sign in first, then we'll bring you straight back here to finish adding your device.
        </Text>
      </Stack>
      {code ? (
        <Stack align="center" marginTop={24}>
          <Text as="p" textRole="pairCode">
            {code}
          </Text>
        </Stack>
      ) : null}
      <Stack direction="row" justify="center" marginTop={32}>
        <Action
          accessibilityRole="link"
          density="touch"
          render={<a href={`/login?redirect=${encodeURIComponent(here)}`} />}
        >
          Sign in
        </Action>
      </Stack>
    </Stack>
  );
};

// An app's sign-in, said plainly BEFORE the click: which app claims to be asking, and the host its
// grant is sent to, which is the part a link from a stranger cannot fake. The server cleans the
// name; the host is the check.
const AppAsking = ({ asking }: { asking: ReturnType<typeof useOAuthRequest> }) => {
  if (asking.isError) {
    const expired = asking.error instanceof ApiError && asking.error.status === 404;
    return (
      <Text as="p" textRole="reading" tone="danger">
        {expired
          ? "This sign-in has expired or was already finished. Start it again from the app."
          : "Couldn't check which app is asking. Reload the page to try again."}
      </Text>
    );
  }
  if (!asking.data)
    return (
      <Text as="p" textRole="reading" tone="secondary">
        Checking which app is asking…
      </Text>
    );
  const { app, sendsTo, loopback } = asking.data;
  return (
    <>
      <Text as="p" strong textRole="reading">
        {app} wants to sign in to Loomarr as you.
      </Text>
      <Stack marginTop={8}>
        <Text as="p" textRole="reading" tone="secondary">
          {loopback
            ? `It sends the sign-in to an app on the computer you're using (${sendsTo}).`
            : `It sends the sign-in to ${sendsTo}.`}{" "}
          Add it only if you just asked it to connect: it can do everything you can in Loomarr until you
          remove it.
        </Text>
      </Stack>
    </>
  );
};

// The three distinct causes `approve.isError` can hide behind. A 404 covers both a mistyped code
// and a dead one *on purpose* — `handleDeviceApprove` (internal/api/deviceroutes.go) returns the
// identical response for "wrong" and "expired" so the error can't be used to grind codes — so it
// keeps today's message unchanged. 429 (the per-user+per-IP `deviceLimiter`) and anything else
// (a dropped request, a 5xx) are real, distinct causes and get their own copy.
type ApproveFailure = "invalid-code" | "rate-limited" | "unreachable";

const classifyApproveFailure = (error: unknown): ApproveFailure => {
  if (error instanceof ApiError && error.status === 429) return "rate-limited";
  if (error instanceof ApiError && error.status === 404) return "invalid-code";
  return "unreachable";
};

const PairDevice = ({ initialCode, next }: PairDeviceProps) => {
  // Prefilled but NEVER auto-submitted. `?code=` is RFC 8628's verification_uri_complete — what a
  // QR encodes — and approving on load would mean any link a person opens could pair a device
  // silently. The click is the consent, so the link only saves typing.
  const [approved, setApproved] = useState<string | null>(null);
  const queryClient = useQueryClient();
  const { user, isLoading } = useAuth();
  // Read only once signed in: the route needs a session, and signed-out visitors see SignInFirst.
  const asking = useOAuthRequest(user ? next : undefined);

  const approve = authApi.useDevicePairApprove({
    mutation: {
      onSuccess: () => {
        void queryClient.invalidateQueries({ queryKey: authApi.getDeviceListQueryKey() });
      },
    },
  });

  const attempt = (userCode: string) => {
    approve.mutate(
      { data: { userCode } },
      {
        onSuccess: (res) => {
          // An app's sign-in finishes on the server, which sends this browser back to the app.
          if (next) {
            window.location.assign(next);
            return;
          }
          const body = unwrap(res);
          setApproved(body?.deviceName ?? "Device");
          form.setFieldValue("userCode", "");
        },
      },
    );
  };

  // A blank code never reaches the server: the schema blocks it, and the button is disabled for
  // it anyway, so there is no message to show.
  const form = useForm({
    defaultValues: { userCode: initialCode ?? "" },
    validators: { onSubmit: devicePairApproveSchema },
    onSubmit: ({ value }) => attempt(value.userCode),
  });

  // A dropped request or a 5xx is worth retrying with the exact same code — unlike a bad or
  // expired one, nothing about the code itself was the problem.
  const retry = () => attempt(form.state.values.userCode);

  if (isLoading) return null;
  if (!user) return <SignInFirst code={initialCode ?? ""} next={next} />;

  return (
    <Stack marginHorizontal="auto" maxWidth={512} paddingHorizontal={24} paddingVertical={64} width="100%">
      <Text as="h1" strong textRole="display">
        Add a device
      </Text>
      <Stack marginTop={12}>
        {next ? (
          <AppAsking asking={asking} />
        ) : (
          <Text as="p" textRole="reading" tone="secondary">
            Your TV, streaming box or phone is showing a short code. Enter it here and it signs in as you.
          </Text>
        )}
      </Stack>

      <Stack
        gap="4"
        marginTop={32}
        noValidate
        onSubmit={(event) => {
          event.preventDefault();
          void form.handleSubmit();
        }}
        render="form"
      >
        <form.Field name="userCode">
          {(field) => (
            <Field
              accessibilityLabel="Code shown on the device"
              // ⚠ The mobile keyboard hints are not polish. This page is opened on a phone almost
              // every time, and without them iOS offers autocorrect and a lowercase keyboard for a
              // code that is always uppercase — producing "wrong code" errors that are really
              // keyboard errors, on a value the user copied correctly.
              autoCapitalize="characters"
              autoComplete="one-time-code"
              autoCorrect={false}
              autoFocus
              maxLength={9}
              onChangeText={field.handleChange}
              placeholder="BCDF-GHJK"
              spellCheck={false}
              style={{
                fontSize: 30,
                height: 80,
                letterSpacing: 10,
                textAlign: "center",
                textTransform: "uppercase",
              }}
              value={field.state.value}
            />
          )}
        </form.Field>
        <form.Subscribe selector={(state) => state.values.userCode}>
          {(userCode) => (
            <Action
              density="touch"
              // An app's sign-in cannot be approved until the page has said which app it is.
              disabled={!userCode.trim() || approve.isPending || (next !== undefined && !asking.data)}
              style={{ width: "100%" }}
              submit
            >
              {approve.isPending ? "Checking…" : "Add device"}
            </Action>
          )}
        </form.Subscribe>
      </Stack>

      {approve.isError
        ? (() => {
            const failure = classifyApproveFailure(approve.error);
            if (failure === "invalid-code")
              return (
                <Stack marginTop={16}>
                  <Text as="p" textAlign="center" textRole="reading" tone="danger">
                    That code is wrong or has expired. Codes last ten minutes — check the screen and try
                    again.
                  </Text>
                </Stack>
              );
            if (failure === "rate-limited")
              return (
                <Stack marginTop={16}>
                  <Text as="p" textAlign="center" textRole="reading" tone="warning">
                    Too many pairing codes tried. Wait a moment and try again.
                  </Text>
                </Stack>
              );
            return (
              <Stack marginTop={16}>
                <Text as="p" textAlign="center" textRole="reading" tone="warning">
                  Couldn't reach Loomarr. Check your connection and try again.
                </Text>
                <Stack direction="row" justify="center" marginTop={12}>
                  <Action onPress={retry} variant="outline">
                    Try again
                  </Action>
                </Stack>
              </Stack>
            );
          })()
        : null}

      {approved ? (
        <Stack borderColor="$borderDecorative" borderRadius={8} borderWidth={1} marginTop={32} padding={20}>
          <Text as="p" textAlign="center" textRole="heading">
            {approved} is ready
          </Text>
          <Stack marginTop={4}>
            <Text as="p" textAlign="center" textRole="body" tone="secondary">
              It should start playing in a few seconds. You can remove it any time from Settings → Access and
              devices.
            </Text>
          </Stack>
        </Stack>
      ) : null}

      {/* A paired device acts as the person who approved it, with their real role (ADR 0043), so an
          admin's TV is an admin. */}
      <Stack marginTop={40}>
        <Text as="p" textAlign="center" textRole="compact" tone="secondary">
          The device acts with your access and role. Removing it later signs out only that device.
        </Text>
      </Stack>
    </Stack>
  );
};

export { PairDevice };
