import { type CreateChannelInputBodyStrategy, channelsApi, toProblem } from "@loomarr/api";
import { useState } from "react";
import { toast } from "sonner";
import {
  Button,
  Card,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui";
import type { ChannelCreateDialogProps } from "./channel-create-dialog.type";

// The three ordering strategies a channel can start with (the DTO enum). Sequential is the
// default — the plainest "play the lineup in order" behavior; the programming rules on the
// channel's own page tune the rest later.
const STRATEGIES: { value: CreateChannelInputBodyStrategy; label: string }[] = [
  { value: "sequential", label: "In order" },
  { value: "shuffle", label: "Shuffled" },
  { value: "time_slot", label: "Time slots" },
];

// ChannelCreateDialog — the "New channel" action's form: make an EMPTY (hand-made) channel
// with just a name + number + strategy, then hand its id back so the list can drop you on
// its page (where Refine-with-AI + the manual lineup editor fill it). The channel `id` is
// omitted — the server assigns one (§7), so there is no client-side id scheme. This is the
// deliberate "or hand-made" create path; the describe-a-channel (Suggest) path stays the
// headline. Inline Card, same open/close idiom as ClipTagDialog (not a fixed overlay).
const ChannelCreateDialog = ({ onCreated, onClose }: ChannelCreateDialogProps) => {
  const [name, setName] = useState("");
  const [number, setNumber] = useState("");
  const [strategy, setStrategy] = useState<CreateChannelInputBodyStrategy>("sequential");

  const create = channelsApi.useCreateChannel({
    mutation: {
      onSuccess: (res) => {
        if (res.status === 200) {
          toast.success("Channel created");
          onCreated(res.data.id);
        }
      },
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't create the channel"),
    },
  });

  const num = Number(number);
  const valid = name.trim().length > 0 && Number.isInteger(num) && num >= 1;

  const submit = () => {
    if (!valid) return;
    // No `id` — the server assigns one and returns it (§7). name/number/strategy only:
    // an empty channel you then fill on its page.
    create.mutate({ data: { name: name.trim(), number: num, strategy } });
  };

  return (
    <Card>
      <section aria-label="Create a channel" className="flex flex-col gap-4 p-4">
        <div className="flex items-start justify-between gap-3">
          <div>
            <h2 className="font-semibold text-lg">New channel</h2>
            <p className="mt-1 text-muted-foreground text-sm">
              Start an empty channel, then fill it on its page — describe it with AI, or add titles by hand.
            </p>
          </div>
          <Button variant="ghost" size="sm" onClick={onClose}>
            Close
          </Button>
        </div>

        <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
          <div className="flex-1">
            <Label htmlFor="new-channel-name">Name</Label>
            <Input
              id="new-channel-name"
              value={name}
              placeholder="Saturday Morning Cartoons"
              autoComplete="off"
              disabled={create.isPending}
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <div className="w-24">
            <Label htmlFor="new-channel-number">Number</Label>
            <Input
              id="new-channel-number"
              type="number"
              min={1}
              value={number}
              placeholder="42"
              disabled={create.isPending}
              onChange={(e) => setNumber(e.target.value)}
            />
          </div>
          <div className="w-40">
            <Label htmlFor="new-channel-strategy">Ordering</Label>
            <Select value={strategy} onValueChange={(v) => setStrategy(v as CreateChannelInputBodyStrategy)}>
              <SelectTrigger id="new-channel-strategy">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {STRATEGIES.map((s) => (
                  <SelectItem key={s.value} value={s.value}>
                    {s.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>

        {/* A duplicate number (or any other create error) surfaces via the toast; the inline
            line names the common one so the fix ("pick another number") is obvious without
            reading the toast. */}
        {create.error != null && (
          <p className="text-onair-300 text-sm">
            {toProblem(create.error).title ?? "Couldn't create the channel."}
            {toProblem(create.error).status === 409 ? " That channel number is already in use." : ""}
          </p>
        )}

        <div className="flex items-center gap-2">
          <Button variant="suggest" size="sm" disabled={!valid || create.isPending} onClick={submit}>
            {create.isPending ? "Creating…" : "Create channel"}
          </Button>
          <Button variant="ghost" size="sm" disabled={create.isPending} onClick={onClose}>
            Cancel
          </Button>
        </div>
      </section>
    </Card>
  );
};

export { ChannelCreateDialog };
