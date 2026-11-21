import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AccessTokenSection from "@/components/Settings/AccessTokenSection";

const { copy, listPersonalAccessTokens, profile } = vi.hoisted(() => ({
  copy: vi.fn(),
  listPersonalAccessTokens: vi.fn(),
  profile: { instanceUrl: "" },
}));

vi.mock("copy-to-clipboard", () => ({ default: copy }));
vi.mock("@/connect", () => ({ userServiceClient: { listPersonalAccessTokens } }));
vi.mock("@/contexts/InstanceContext", () => ({ useInstance: () => ({ profile }) }));
vi.mock("@/hooks/useCurrentUser", () => ({ default: () => ({ name: "users/note-owner" }) }));

describe("MCP and REST setup in access token settings", () => {
  beforeEach(() => {
    copy.mockResolvedValue(true);
    listPersonalAccessTokens.mockResolvedValue({ personalAccessTokens: [] });
    profile.instanceUrl = "";
  });

  it("copies connection examples using the canonical instance URL and a token placeholder", async () => {
    profile.instanceUrl = "https://notes.example.com/notes";
    render(<AccessTokenSection />);
    await screen.findByRole("heading", { name: "MCP & API" });

    fireEvent.click(screen.getByRole("button", { name: "Copy MCP client configuration" }));
    expect(JSON.parse(copy.mock.lastCall?.[0])).toEqual({
      mcpServers: {
        "cat-two": {
          type: "http",
          url: "https://notes.example.com/notes/mcp",
          headers: { Authorization: "Bearer <YOUR_PAT>" },
        },
      },
    });

    fireEvent.click(screen.getByRole("button", { name: "Copy Create a private test note" }));
    expect(copy.mock.lastCall?.[0]).toContain("POST 'https://notes.example.com/notes/api/memos'");
    expect(copy.mock.lastCall?.[0]).toContain("Authorization: Bearer <YOUR_PAT>");
    expect(copy.mock.lastCall?.[0]).toContain('"visibility":"PRIVATE"');
  });

  it("copies direct endpoints for the current origin with no canonical URL", async () => {
    render(<AccessTokenSection />);
    await screen.findByRole("heading", { name: "MCP & API" });

    fireEvent.click(screen.getByRole("button", { name: "Copy MCP server URL" }));
    expect(copy).toHaveBeenLastCalledWith(`${window.location.origin}/mcp`);
    fireEvent.click(screen.getByRole("button", { name: "Copy REST API base URL" }));
    expect(copy).toHaveBeenLastCalledWith(`${window.location.origin}/api`);
    expect(listPersonalAccessTokens).toHaveBeenCalledWith({ parent: "users/note-owner" });
  });
});
