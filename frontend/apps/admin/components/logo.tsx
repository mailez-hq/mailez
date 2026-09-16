import { cn } from "@/lib/utils";
import { BASE_PATH } from "@/lib/api";

// Logo renders the mailez brand icon as a rounded tile.
export function Logo({ className }: { className?: string }) {
  return (
    <img
      src={`${BASE_PATH}/mailez-icon.svg`}
      alt="Mailez"
      className={cn("size-8 shrink-0 rounded-lg", className)}
    />
  );
}
