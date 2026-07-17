import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

// The shadcn class-merge helper: clsx for conditional composition, tailwind-merge
// to resolve conflicting utilities so later wins (e.g. `px-2` + `px-4` → `px-4`).
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs));
}
