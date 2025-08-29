"use client";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

// PasswordSection is its own <form> with its own submit (savePassword), so
// it does not share the profile form's save bar.
export function PasswordSection({
  t,
  oldPw, setOldPw,
  newPw, setNewPw,
  confirmPw, setConfirmPw,
  savePassword,
}: {
  t: (key: string) => string;
  oldPw: string; setOldPw: (v: string) => void;
  newPw: string; setNewPw: (v: string) => void;
  confirmPw: string; setConfirmPw: (v: string) => void;
  savePassword: (e: React.FormEvent) => void;
}) {
  return (
    <form onSubmit={savePassword} className="h-full space-y-4 overflow-y-auto p-5">
      <p className="text-sm font-medium">{t("changePassword")}</p>
      <div className="space-y-2">
        <Label>{t("currentPassword")}</Label>
        <Input type="password" value={oldPw} onChange={(e) => setOldPw(e.target.value)} required />
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label>{t("newPassword")}</Label>
          <Input type="password" value={newPw} onChange={(e) => setNewPw(e.target.value)} required />
        </div>
        <div className="space-y-2">
          <Label>{t("confirmPassword")}</Label>
          <Input type="password" value={confirmPw} onChange={(e) => setConfirmPw(e.target.value)} required />
        </div>
      </div>
      <div className="flex justify-end">
        <Button type="submit">{t("changePasswordBtn")}</Button>
      </div>
    </form>
  );
}
