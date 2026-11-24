import { create } from "@bufbuild/protobuf";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { trashQueryKey } from "@/lib/trash";
import Trash from "@/pages/Trash";
import { MemoSchema } from "@/types/proto/api/memo_service_pb";
import { UserSetting_TagsSettingSchema } from "@/types/proto/api/user_service_pb";

const auth = vi.hoisted(() => ({ isUserSettingsInitialized: false }));
const trash = vi.hoisted(() => ({ listTrash: vi.fn(), changeTrash: vi.fn() }));

vi.mock("@/contexts/AuthContext", () => ({
  useAuth: () => ({
    currentUser: { name: "users/admin" },
    isUserSettingsInitialized: auth.isUserSettingsInitialized,
    userSettingsLoadError: false,
    userTagsSetting: auth.isUserSettingsInitialized
      ? create(UserSetting_TagsSettingSchema, { tags: { private: { blurContent: true } } })
      : undefined,
  }),
}));
vi.mock("@/components/MemoEditor/MemoMarkdown", () => ({ MemoMarkdown: ({ value }: { value: string }) => <p>{value}</p> }));
vi.mock("@/lib/trash", () => ({ ...trash, trashQueryKey: ["trash"] }));

describe("Trash content privacy", () => {
  it("waits for display settings, then reveals matching previews only by request without blocking restore", async () => {
    auth.isUserSettingsInitialized = false;
    const deletedTime = "2026-11-21T16:00:00Z";
    const sensitive = create(MemoSchema, {
      name: "memos/private",
      content: "Sensitive draft",
      tags: ["private"],
      attachments: [{ name: "attachments/statement", filename: "Bank statement.pdf" }],
    });
    const ordinary = create(MemoSchema, { name: "memos/ordinary", content: "Ordinary draft" });
    const page = {
      memos: [
        { memo: sensitive, deletedTime },
        { memo: ordinary, deletedTime },
      ],
      nextPageToken: "",
    };
    trash.listTrash.mockResolvedValue(page);
    trash.changeTrash.mockResolvedValue(undefined);
    const client = new QueryClient();
    client.setQueryData([...trashQueryKey, "users/admin"], { pages: [page], pageParams: [""] });
    const pageContent = () => (
      <QueryClientProvider client={client}>
        <Trash />
      </QueryClientProvider>
    );
    const { rerender } = render(pageContent());

    expect(screen.getByRole("status")).toHaveTextContent("Loading Trash");
    expect(screen.queryByText(ordinary.content)).not.toBeInTheDocument();
    expect(screen.queryByText(sensitive.content)).not.toBeInTheDocument();
    expect(screen.queryByText("Bank statement.pdf")).not.toBeInTheDocument();

    auth.isUserSettingsInitialized = true;
    rerender(pageContent());

    expect(screen.getByText(ordinary.content)).toBeVisible();
    expect(screen.queryByText(sensitive.content)).not.toBeInTheDocument();
    expect(screen.queryByText("Bank statement.pdf")).not.toBeInTheDocument();
    const reveal = screen.getByRole("button", { name: "Click to show sensitive content" });
    expect(reveal).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(reveal);
    expect(screen.getByText(sensitive.content)).toBeVisible();
    expect(screen.getByText("Bank statement.pdf")).toBeVisible();
    fireEvent.click(reveal);
    expect(reveal).toHaveAttribute("aria-expanded", "false");

    const sensitiveRow = screen.getAllByRole("article")[0];
    fireEvent.click(within(sensitiveRow).getByRole("button", { name: "Restore" }));
    await waitFor(() => expect(trash.changeTrash).toHaveBeenCalledWith(sensitive.name, "restore"));
  });
});
