import { cn } from "@/lib/utils";

// Logo renders the mailez brand icon as a rounded tile.
export function Logo({ className }: { className?: string }) {
  return (
    <img
      src="/mailez-icon.svg"
      alt="mailez"
      className={cn("size-8 shrink-0 rounded-lg", className)}
    />
  );
}
