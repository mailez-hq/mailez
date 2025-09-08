"use client";

import { Component, type ReactNode } from "react";

// Route-level render throws replace the whole page with Next's global error
// screen ("This page couldn't load"). A reading-pane crash is a data-shaped
// problem for ONE message — it must cost one pane, not the whole route.
// This boundary catches render throws from the reader subtree and shows an
// in-place card; it resets automatically when another message is opened.
type Props = {
  children: ReactNode;
  // Changing this (opened message changed) clears a caught error.
  resetKey: string | number;
};
type State = { error: Error | null };

export class PaneErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidUpdate(prev: Props) {
    if (prev.resetKey !== this.props.resetKey && this.state.error) {
      this.setState({ error: null });
    }
  }

  render() {
    if (this.state.error) {
      return (
        <div className="flex h-full flex-col items-center justify-center gap-3 p-8 text-center">
          <p className="text-sm font-medium text-foreground">
            This message could not be displayed
          </p>
          <p className="max-w-md truncate font-mono text-xs text-muted-foreground">
            {String(this.state.error?.message || this.state.error)}
          </p>
          <div className="flex gap-2">
            <button
              type="button"
              onClick={() => this.setState({ error: null })}
              className="rounded-md border px-3 py-1.5 text-xs hover:bg-accent"
            >
              Retry
            </button>
            <button
              type="button"
              onClick={() => window.history.back()}
              className="rounded-md border px-3 py-1.5 text-xs hover:bg-accent"
            >
              Back
            </button>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}
