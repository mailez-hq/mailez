import { MailView } from "@/components/mailbox/mail-view";

// MailView renders the three-pane message UI for the whole /mail segment and
// stays mounted across folder / message transitions so the list never
// rebuilds. Leaf routes ([folder], [id]) are thin stubs; the URL is the
// single source for folder & opened uid.
export default function MailAreaLayout({ children }: { children: React.ReactNode }) {
  return (
    <>
      <MailView />
      <div className="hidden">{children}</div>
    </>
  );
}
