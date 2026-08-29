import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { MailMessage } from "@mailez/types";

import { ThreadMessage } from "@/components/mailbox/reader/thread-message";

afterEach(cleanup);

function makeMessage(over: Partial<MailMessage> = {}): MailMessage {
  return {
    uid: 1,
    id: "m1",
    seq: 1,
    subject: "Quarterly report",
    from: [{ name: "Bob", email: "bob@x.com" }],
    to: [{ name: "Alice", email: "alice@example.com" }],
    date: "2026-01-01T10:30:00Z",
    flags: ["\\Seen"],
    has_attachment: false,
    text_body: "Hello Alice,\n\nBudget numbers inside.\n\n-- Bob",
    html_body: "",
    ...over,
  };
}

function renderThreadMessage(message: MailMessage, over: { remoteLoaded?: boolean } = {}) {
  return render(
    <ThreadMessage
      message={message}
      isExpanded={true}
      onToggleExpand={vi.fn()}
      onClose={vi.fn()}
      detailsOpen={false}
      onToggleDetails={vi.fn()}
      expandedQuotes={new Set()}
      onToggleQuote={vi.fn()}
      remoteLoaded={over.remoteLoaded ?? false}
      onReply={vi.fn()}
      onReplyAll={vi.fn()}
      onForward={vi.fn()}
    />,
  );
}

describe("ThreadMessage", () => {
  it("renders sender and text body", () => {
    renderThreadMessage(makeMessage());
    expect(screen.getByText("Bob")).toBeInTheDocument();
    expect(screen.getByText(/Budget numbers inside/)).toBeInTheDocument();
  });

  it("strips active content from the html body before rendering", () => {
    renderThreadMessage(makeMessage({
      html_body:
        '<p>Hi <b>friend</b></p><script>window.__pwned=1</script>' +
        '<img src="x" onerror="window.__pwned=2">' +
        '<a href="javascript:window.__pwned=3">click</a>' +
        '<iframe src="https://evil.example"></iframe>',
    }));

    // Benign markup survives sanitization...
    expect(screen.getByText(/friend/)).toBeInTheDocument();

    // ...while active content is gone: no script/iframe nodes, no inline
    // handlers, no javascript: URLs, and nothing ever executed.
    expect(document.querySelector("script")).toBeNull();
    expect(document.querySelector("iframe")).toBeNull();
    const img = document.querySelector("img");
    if (img) expect(img.getAttribute("onerror")).toBeNull();
    const link = document.querySelector("a");
    const href = link?.getAttribute("href");
    if (href) expect(href).not.toMatch(/^javascript:/i);
    expect((window as unknown as { __pwned?: unknown }).__pwned).toBeUndefined();
  });

  it("keeps remote images blocked (placeholder src + data-remote-src) until opt-in", () => {
    renderThreadMessage(makeMessage({
      html_body: '<p>newsletter</p><img src="https://tracker.example/pixel.png">',
    }));

    // The raw remote src is not loaded...
    expect(document.querySelector('img[src="https://tracker.example/pixel.png"]')).toBeNull();
    // ...but the original URL is preserved for the "Load images" action.
    const img = document.querySelector('img[data-remote-src="https://tracker.example/pixel.png"]');
    expect(img).not.toBeNull();
    expect(img?.getAttribute("src")).toBe("about:blank");
  });

  it("loads remote images once the reader opts in", () => {
    renderThreadMessage(makeMessage({
      html_body: '<p>newsletter</p><img src="https://tracker.example/pixel.png">',
    }), { remoteLoaded: true });
    expect(document.querySelector('img[src="https://tracker.example/pixel.png"]')).not.toBeNull();
  });

  it("wires the reply buttons to their callbacks", () => {
    const onReply = vi.fn();
    const onReplyAll = vi.fn();
    const onForward = vi.fn();
    render(
      <ThreadMessage
        message={makeMessage()}
        isExpanded={true}
        onToggleExpand={vi.fn()}
        onClose={vi.fn()}
        detailsOpen={false}
        onToggleDetails={vi.fn()}
        expandedQuotes={new Set()}
        onToggleQuote={vi.fn()}
        remoteLoaded={false}
        onReply={onReply}
        onReplyAll={onReplyAll}
        onForward={onForward}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "reply" }));
    fireEvent.click(screen.getByRole("button", { name: "replyAll" }));
    fireEvent.click(screen.getByRole("button", { name: "forward" }));
    expect(onReply).toHaveBeenCalledTimes(1);
    expect(onReplyAll).toHaveBeenCalledTimes(1);
    expect(onForward).toHaveBeenCalledTimes(1);
  });
});
