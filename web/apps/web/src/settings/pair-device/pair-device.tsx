import * as authApi from "@loomarr/api/endpoints/auth";
import { unwrap } from "@loomarr/api/unwrap";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useAuth } from "@/auth/use-auth";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

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
};

// Signed-out is an EXPECTED arrival state here, not an error: the person holding the remote is
// often not yet signed in on the phone they are typing on. So the page says what to do and hands
// them a link back to this exact URL — code included — rather than a bare "unauthorized".
const SignInFirst = ({ code }: { code: string }) => {
  const here = `/pair${code ? `?code=${encodeURIComponent(code)}` : ""}`;
  return (
    <div className="mx-auto max-w-lg px-6 py-16 text-center">
      <h1 className="font-semibold text-3xl">Add a device</h1>
      <p className="mt-3 text-lg text-muted-foreground leading-relaxed">
        Sign in first, then we'll bring you straight back here to finish adding your TV.
      </p>
      {code ? <p className="mt-6 font-mono text-2xl tracking-[0.3em]">{code}</p> : null}
      <Button
        className="mt-8 h-14 px-8 text-lg"
        render={<a href={`/login?redirect=${encodeURIComponent(here)}`}>Sign in</a>}
        size="lg"
      />
    </div>
  );
};

const PairDevice = ({ initialCode }: PairDeviceProps) => {
  // Prefilled but NEVER auto-submitted. `?code=` is RFC 8628's verification_uri_complete — what a
  // QR encodes — and approving on load would mean any link a person opens could pair a device
  // silently. The click is the consent, so the link only saves typing.
  const [code, setCode] = useState(initialCode ?? "");
  const [approved, setApproved] = useState<string | null>(null);
  const queryClient = useQueryClient();
  const { user, isLoading } = useAuth();

  const approve = authApi.useDevicePairApprove({
    mutation: {
      onSuccess: () => {
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

  if (isLoading) return null;
  if (!user) return <SignInFirst code={initialCode ?? ""} />;

  return (
    <div className="mx-auto max-w-lg px-6 py-16">
      <h1 className="font-semibold text-3xl">Add a device</h1>
      <p className="mt-3 text-lg text-muted-foreground leading-relaxed">
        Your TV or streaming box is showing a short code. Enter it here and it signs in as you.
      </p>

      <form className="mt-8 space-y-4" onSubmit={submit}>
        <Input
          aria-label="Code shown on the device"
          // ⚠ The mobile keyboard hints are not polish. This page is opened on a phone almost
          // every time, and without them iOS offers autocorrect and a lowercase keyboard for a
          // code that is always uppercase — producing "wrong code" errors that are really
          // keyboard errors, on a value the user copied correctly.
          autoCapitalize="characters"
          autoComplete="one-time-code"
          autoCorrect="off"
          autoFocus
          className="h-20 w-full text-center font-mono text-3xl uppercase tracking-[0.35em]"
          inputMode="text"
          maxLength={9}
          onChange={(event) => setCode(event.target.value)}
          placeholder="BCDF-GHJK"
          spellCheck={false}
          value={code}
        />
        <Button className="h-14 w-full text-lg" disabled={!code.trim() || approve.isPending} type="submit">
          {approve.isPending ? "Checking…" : "Add device"}
        </Button>
      </form>

      {approve.isError ? (
        <p className="mt-4 text-center text-destructive text-lg">
          That code is wrong or has expired. Codes last ten minutes — check the screen and try again.
        </p>
      ) : null}

      {approved ? (
        <div className="mt-8 rounded-lg border border-border p-5 text-center">
          <p className="font-medium text-lg">{approved} is ready</p>
          <p className="mt-1 text-muted-foreground leading-relaxed">
            It should start playing in a few seconds. You can remove it any time from Settings → Security.
          </p>
        </div>
      ) : null}

      <p className="mt-10 text-center text-muted-foreground text-sm leading-relaxed">
        The device gets your access, not administrator access. Removing it later signs out only that device.
      </p>
    </div>
  );
};

export { PairDevice };
