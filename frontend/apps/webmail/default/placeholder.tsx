"use client";

// Placeholder shared by the optional-module stubs of the default module set.
export function ModulePlaceholder({ label }: { label: string }) {
  return (
    <div className="flex h-full items-center justify-center p-6">
      <div className="max-w-sm rounded-lg border border-dashed border-border p-6 text-center">
        <p className="text-sm font-medium">{label}</p>
        <p className="mt-1 text-xs text-muted-foreground">
          此功能未启用 / This feature is not enabled.
        </p>
      </div>
    </div>
  );
}
