"use client";

// Community-edition placeholder shared by the enterprise-module stubs.
export function EnterprisePlaceholder({ label }: { label: string }) {
  return (
    <div className="flex h-full items-center justify-center p-6">
      <div className="max-w-sm rounded-lg border border-dashed border-border p-6 text-center">
        <p className="text-sm font-medium">{label}</p>
        <p className="mt-1 text-xs text-muted-foreground">
          此功能需要企业版 / This feature requires the enterprise edition.
        </p>
      </div>
    </div>
  );
}
