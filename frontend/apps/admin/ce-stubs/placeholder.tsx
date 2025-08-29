"use client";

// Community-edition placeholder shared by the enterprise-page stubs.
export function EnterprisePagePlaceholder({ label }: { label: string }) {
  return (
    <div className="flex flex-1 items-center justify-center p-8">
      <div className="max-w-md rounded-lg border border-dashed p-8 text-center">
        <p className="text-base font-semibold">{label}</p>
        <p className="mt-2 text-sm text-muted-foreground">
          此功能需要企业版 / This feature requires the enterprise edition.
        </p>
      </div>
    </div>
  );
}
