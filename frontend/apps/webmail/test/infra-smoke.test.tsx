import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { renderWithProviders } from "@/test/test-utils";

describe("test infrastructure", () => {
  it("renders with providers and jest-dom matchers work", () => {
    renderWithProviders(<button type="button">hello</button>);
    expect(screen.getByRole("button", { name: "hello" })).toBeInTheDocument();
  });
});
