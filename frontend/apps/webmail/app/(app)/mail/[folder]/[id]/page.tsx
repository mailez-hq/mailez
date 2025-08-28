// Opened-message view. The reading pane is rendered by MailView (mounted in
// (app)/mail/layout.tsx) from the stable message id in the URL; this leaf
// merely forms the /mail/[folder]/[id] route segment. MailView reads the id
// via usePathname() so folder/index navigation need no extra plumbing here.
export default function MailMessagePage() {
  return null;
}
