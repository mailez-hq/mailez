// Bare /mail without a folder is handled by MailLayout (auth guard), which
// restores the last-visited folder after sign-in. This leaf renders nothing.
export default function MailIndex() {
  return null;
}
