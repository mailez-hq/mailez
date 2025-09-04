"use client";

// Placeholder shared by the optional-page stubs of the default module set.
export function PagePlaceholder({ label }: { label: string }) {
  return (
    <div className="flex flex-1 items-center justify-center p-8">
      <div className="max-w-md rounded-lg border border-dashed p-8 text-center">
        <p className="text-base font-semibold">{label}</p>
        <p className="mt-2 text-sm text-muted-foreground">
          此功能未启用 / This feature is not enabled.
        </p>
      </div>
    </div>
  );
}
