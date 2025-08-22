"use client";

import { useCallback, useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  aiStatus, aiSummarize,
  mailFolders, mailMessage, mailMessages, mailSend, type MailMessage, type Me,
} from "@/lib/api";

export function MailView({ me }: { me: Me }) {
  const [folders, setFolders] = useState<string[]>([]);
  const [folder, setFolder] = useState("INBOX");
  const [messages, setMessages] = useState<MailMessage[]>([]);
  const [selected, setSelected] = useState<MailMessage | null>(null);
  const [detail, setDetail] = useState<MailMessage | null>(null);
  const [composeOpen, setComposeOpen] = useState(false);
  const [error, setError] = useState("");
  const [aiEnabled, setAiEnabled] = useState(false);
  const [summary, setSummary] = useState("");
  const [summarizing, setSummarizing] = useState(false);

  const [to, setTo] = useState("");
  const [subject, setSubject] = useState("");
  const [body, setBody] = useState("");

  const loadFolders = useCallback(async () => {
    try {
      setFolders(await mailFolders());
    } catch (e) {
      setError(e instanceof Error ? e.message : "load folders failed");
    }
  }, []);

  const loadMessages = useCallback(async (f: string) => {
    try {
      setMessages(await mailMessages(f));
    } catch (e) {
      setError(e instanceof Error ? e.message : "load messages failed");
    }
  }, []);

  useEffect(() => {
    loadFolders();
  }, [loadFolders]);

  useEffect(() => {
    loadMessages(folder);
    setSelected(null);
    setDetail(null);
  }, [folder, loadMessages]);

  async function openMessage(m: MailMessage) {
    setSelected(m);
    setDetail(null);
    setSummary("");
    try {
      setDetail(await mailMessage(folder, m.uid));
    } catch (e) {
      setError(e instanceof Error ? e.message : "load message failed");
    }
  }

  async function summarize() {
    if (!detail) return;
    setSummarizing(true);
    setSummary("");
    try {
      const res = await aiSummarize(detail.text_body || detail.html_body || "");
      setSummary(res.summary);
    } catch (e) {
      setError(e instanceof Error ? e.message : "summarize failed");
    } finally {
      setSummarizing(false);
    }
  }

  async function send(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    try {
      await mailSend(to, subject, body);
      setComposeOpen(false);
      setTo(""); setSubject(""); setBody("");
      loadMessages(folder);
    } catch (err) {
      setError(err instanceof Error ? err.message : "send failed");
    }
  }

  const fmtDate = (d: string) =>
    new Date(d).toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });

  return (
    <div className="flex h-screen">
      {/* Folders */}
      <aside className="flex w-44 shrink-0 flex-col border-r bg-white dark:bg-zinc-900">
        <div className="flex items-center justify-between px-3 py-3">
          <span className="text-base font-semibold">mailess</span>
          <Button size="sm" onClick={() => setComposeOpen(true)}>Write</Button>
        </div>
        <ScrollArea className="flex-1">
          <nav className="space-y-1 px-2">
            {folders.map((f) => (
              <button
                key={f}
                onClick={() => setFolder(f)}
                className={`w-full rounded-md px-3 py-1.5 text-left text-sm ${
                  f === folder
                    ? "bg-zinc-100 font-medium text-zinc-900 dark:bg-zinc-800 dark:text-zinc-50"
                    : "text-zinc-600 hover:bg-zinc-100 dark:text-zinc-400 dark:hover:bg-zinc-800"
                }`}
              >
                {f}
              </button>
            ))}
          </nav>
        </ScrollArea>
        <div className="border-t px-3 py-2 text-xs text-zinc-500">{me.email}</div>
      </aside>

      {/* Message list */}
      <div className="flex w-96 shrink-0 flex-col border-r">
        <div className="border-b px-3 py-2 text-sm font-medium text-zinc-500">{folder}</div>
        <ScrollArea className="flex-1">
          {messages.map((m) => (
            <button
              key={m.uid}
              onClick={() => openMessage(m)}
              className={`flex w-full flex-col items-start gap-0.5 border-b px-3 py-2.5 text-left hover:bg-zinc-50 dark:hover:bg-zinc-800/50 ${
                selected?.uid === m.uid ? "bg-zinc-100 dark:bg-zinc-800" : ""
              }`}
            >
              <div className="flex w-full items-baseline justify-between">
                <span className="truncate text-sm font-medium">
                  {m.from[0]?.name || m.from[0]?.email || "unknown"}
                </span>
                <span className="shrink-0 text-xs text-zinc-400">{fmtDate(m.date)}</span>
              </div>
              <span className="truncate text-sm text-zinc-700 dark:text-zinc-300">
                {m.subject || "(no subject)"}
              </span>
            </button>
          ))}
          {messages.length === 0 && (
            <p className="p-4 text-sm text-zinc-400">No messages</p>
          )}
        </ScrollArea>
      </div>

      {/* Reading pane */}
      <main className="flex-1 overflow-y-auto bg-white dark:bg-zinc-950">
        {detail ? (
          <article className="mx-auto max-w-3xl p-6">
            <h1 className="text-2xl font-semibold">{detail.subject}</h1>
            <div className="mt-2 flex items-center gap-2 text-sm text-zinc-500">
              <span className="font-medium text-zinc-900 dark:text-zinc-100">
                {detail.from.map((a) => a.name || a.email).join(", ")}
              </span>
              <span>&lt;{detail.from[0]?.email}&gt;</span>
              <span>·</span>
              <span>{fmtDate(detail.date)}</span>
            </div>
            {aiEnabled && (
              <div className="mt-4">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={summarize}
                  disabled={summarizing}
                >
                  {summarizing ? "Summarizing..." : "AI Summary"}
                </Button>
                {summary && (
                  <div className="mt-2 rounded-md border bg-zinc-50 p-3 text-sm leading-6 dark:bg-zinc-900">
                    <p className="mb-1 text-xs font-medium text-zinc-400">Summary</p>
                    <p className="whitespace-pre-wrap">{summary}</p>
                  </div>
                )}
              </div>
            )}
            <div className="mt-6 whitespace-pre-wrap text-[15px] leading-7">
              {detail.text_body || "(no text body)"}
            </div>
            {detail.html_body && (
              <div className="mt-6 border-t pt-4">
                <p className="mb-2 text-xs text-zinc-400">HTML version:</p>
                <div
                  className="text-sm"
                  dangerouslySetInnerHTML={{ __html: detail.html_body }}
                />
              </div>
            )}
          </article>
        ) : (
          <div className="flex h-full items-center justify-center text-sm text-zinc-400">
            Select a message
          </div>
        )}
      </main>

      {/* Compose dialog */}
      <Dialog open={composeOpen} onOpenChange={setComposeOpen}>
        <DialogContent className="max-w-xl">
          <form onSubmit={send} className="space-y-3">
            <DialogHeader><DialogTitle>New message</DialogTitle></DialogHeader>
            <div className="space-y-1">
              <Label>To</Label>
              <Input value={to} onChange={(e) => setTo(e.target.value)} placeholder="user@example.com" required />
            </div>
            <div className="space-y-1">
              <Label>Subject</Label>
              <Input value={subject} onChange={(e) => setSubject(e.target.value)} />
            </div>
            <div className="space-y-1">
              <Label>Body</Label>
              <Textarea value={body} onChange={(e) => setBody(e.target.value)} rows={8} placeholder="Write your message..." />
            </div>
            {error && <p className="text-sm text-red-600">{error}</p>}
            <DialogFooter><Button type="submit">Send</Button></DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
