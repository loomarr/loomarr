import type { Intent } from "@loomarr/api";
import { CHANNEL_TEMPLATES, intentSchema } from "@loomarr/core";
import { useState } from "react";
import { IntentInput } from "@/components/loomarr";
import { Input, Label } from "@/components/ui";
import type { IntentFormProps } from "./intent-form.type";

// The hero (§3) plus the constraints §13 calls "intent-writing hints": era, tone,
// runtime target, and an acquisition cap. They stay behind a disclosure because the
// blank-page problem is solved by ONE good sentence, not by a form — but they are here,
// and they now actually reach the server (`runtimeTargetMin` was unreachable until the
// submit body was typed from the domain).
const IntentForm = ({ initialDescription = "", onSubmit, submitting = false }: IntentFormProps) => {
  const [description, setDescription] = useState(initialDescription);
  const [refining, setRefining] = useState(false);
  const [era, setEra] = useState("");
  const [tone, setTone] = useState("");
  const [runtime, setRuntime] = useState("");
  const [maxAcq, setMaxAcq] = useState("");
  // mustInclude/mustExclude have been in the shared schema and consumed by the scorer
  // (score.go weights a must-include match) since the beginning — with no way to set
  // them short of hand-crafting an API call. Comma-separated because these are a few
  // titles or terms, not a managed list; a tag editor would be more UI than the job needs.
  const [mustInclude, setMustInclude] = useState("");
  const [mustExclude, setMustExclude] = useState("");

  // "" → undefined (omitted), never [] — an empty array is a claim ("nothing required"),
  // absence is the truth ("the user didn't say").
  const terms = (v: string): string[] | undefined => {
    const list = v
      .split(",")
      .map((t) => t.trim())
      .filter(Boolean);
    return list.length > 0 ? list : undefined;
  };

  const submit = () => {
    // The schema is the same one packages/core shares with mobile, and its field names
    // are the wire's (guarded by intent-contract.test.ts).
    const parsed = intentSchema.safeParse({
      description,
      era: era.trim() || undefined,
      tone: tone.trim() || undefined,
      runtimeTargetMin: runtime ? Number(runtime) : undefined,
      maxAcquisitions: maxAcq ? Number(maxAcq) : undefined,
      mustInclude: terms(mustInclude),
      mustExclude: terms(mustExclude),
    });
    if (parsed.success) onSubmit(parsed.data as Intent);
  };

  return (
    <div className="flex flex-col gap-4">
      <IntentInput
        value={description}
        onValueChange={setDescription}
        onSubmit={submit}
        submitting={submitting}
        templates={CHANNEL_TEMPLATES.map((t) => ({ label: t.label, value: t.description }))}
      />

      <button
        type="button"
        onClick={() => setRefining((v) => !v)}
        className="w-fit cursor-pointer text-muted-foreground text-sm underline-offset-4 hover:text-foreground hover:underline"
      >
        {refining ? "Hide constraints" : "Add constraints"}
      </button>

      {refining && (
        <div className="grid grid-cols-2 gap-3">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="intent-era">Era</Label>
            <Input id="intent-era" value={era} placeholder="1990s" onChange={(e) => setEra(e.target.value)} />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="intent-tone">Tone</Label>
            <Input
              id="intent-tone"
              value={tone}
              placeholder="high-energy"
              onChange={(e) => setTone(e.target.value)}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="intent-runtime">Target runtime (minutes)</Label>
            <Input
              id="intent-runtime"
              type="number"
              value={runtime}
              placeholder="180"
              onChange={(e) => setRuntime(e.target.value)}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="intent-max">Max titles to acquire</Label>
            <Input
              id="intent-max"
              type="number"
              value={maxAcq}
              placeholder="10"
              onChange={(e) => setMaxAcq(e.target.value)}
            />
          </div>
          <div className="col-span-2 flex flex-col gap-1.5">
            <Label htmlFor="intent-must-include">Must include</Label>
            <Input
              id="intent-must-include"
              value={mustInclude}
              placeholder="Point Break, Heat"
              onChange={(e) => setMustInclude(e.target.value)}
            />
          </div>
          <div className="col-span-2 flex flex-col gap-1.5">
            <Label htmlFor="intent-must-exclude">Must exclude</Label>
            <Input
              id="intent-must-exclude"
              value={mustExclude}
              placeholder="anything with clowns"
              onChange={(e) => setMustExclude(e.target.value)}
            />
          </div>
        </div>
      )}
    </div>
  );
};

export { IntentForm };
