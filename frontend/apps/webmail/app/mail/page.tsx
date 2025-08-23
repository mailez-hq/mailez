import { redirect } from "next/navigation";

export default function MailIndex() {
  redirect("/mail/INBOX");
}