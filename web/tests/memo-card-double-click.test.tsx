import { create } from "@bufbuild/protobuf";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { createPortal } from "react-dom";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { cacheService } from "@/components/MemoEditor/edit/services/cacheService";
import { useEditorSelector } from "@/components/MemoEditor/edit/state";
import MemoView from "@/components/MemoView";
import { DEVICE_PREFERENCES_KEY } from "@/hooks/useDevicePreferences";
import { State } from "@/types/proto/api/common_pb";
import { MemoSchema } from "@/types/proto/api/memo_service_pb";

const owner = "users/owner";
const memo = create(MemoSchema, {
  name: "memos/one",
  creator: owner,
  content: "Plain **text** @owner #work [website](https://example.org)\n\n- [ ] Todo",
  revision: "r1",
  tags: ["work"],
});
const otherMemo = create(MemoSchema, { name: "memos/two", creator: owner, content: "Second note", revision: "r2" });
const viewer = vi.hoisted(() => ({ name: "", state: 0, blur: false }));

vi.mock("@/hooks/useCurrentUser", () => ({ default: () => viewer }));
vi.mock("@/contexts/AuthContext", () => ({ useAuth: () => ({ userTagsSetting: { tags: { work: { blurContent: viewer.blur } } } }) }));
vi.mock("@/contexts/MentionResolutionContext", () => ({ useResolvedUser: () => undefined, useResolvedMentionUsernames: () => new Set() }));
vi.mock("@/components/MemoEditor/view/useTagNavigation", () => ({ useTagNavigation: () => () => {} }));
vi.mock("@/hooks/useMemoQueries", () => ({
  useMemoByName: (name: string) => ({ data: [memo, otherMemo].find((item) => item.name === name) }),
  useUpdateMemo: () => ({ mutateAsync: vi.fn() }),
}));
vi.mock("@/components/MemoView/components/MemoHeader", () => ({
  default: () => createPortal(<p>Dialog preview</p>, document.body),
}));
vi.mock("@/components/MemoMetadata", () => ({ AttachmentGallery: () => null, MemoMetadataRows: () => null }));
vi.mock("@/components/MemoReactionListView", () => ({ MemoReactionListView: () => null }));
vi.mock("@/components/MemoEditor/edit/loadEditSession", () => ({
  getEditSession: () => EditSession,
}));

function EditSession({ onCancel }: { onCancel: () => void }) {
  const content = useEditorSelector((draft) => draft.content);
  return (
    <>
      <textarea aria-label="Edit memo" defaultValue={content} />
      <button type="button" onClick={onCancel}>
        Cancel
      </button>
    </>
  );
}

function renderCards(memos = [memo]) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <main>
          {memos.map((item) => (
            <MemoView key={item.name} memo={item} />
          ))}
        </main>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return screen.getAllByRole("article");
}

beforeEach(() => {
  localStorage.clear();
  Object.assign(viewer, { name: owner, state: State.NORMAL, blur: false });
});

describe("inline memo editing", () => {
  it("opens the existing draft in the same card from text or padding", () => {
    cacheService.saveNow(cacheService.key(owner, "edit:memos/one"), {
      content: "Unsaved draft",
      baseRevision: memo.revision,
      attachments: [],
    });
    const [card] = renderCards();
    fireEvent.doubleClick(within(card).getByText("text"));
    const field = within(card).getByRole("textbox", { name: "Edit memo" });
    expect(field).toHaveValue("Unsaved draft");
    fireEvent.change(field, { target: { value: "Still editing" } });
    fireEvent.doubleClick(field);
    expect(field).toHaveValue("Still editing");
    fireEvent.click(within(card).getByRole("button", { name: "Cancel" }));
    fireEvent.doubleClick(card);
    expect(within(card).getByRole("textbox", { name: "Edit memo" })).toBeInTheDocument();
  });

  it("ignores list whitespace and opens only the card that received the double-click", () => {
    const [first, second] = renderCards([memo, otherMemo]);
    fireEvent.doubleClick(screen.getByRole("main"));
    expect(screen.queryByRole("textbox", { name: "Edit memo" })).not.toBeInTheDocument();

    fireEvent.doubleClick(within(second).getByText(otherMemo.content));
    expect(within(second).getByRole("textbox", { name: "Edit memo" })).toHaveValue(otherMemo.content);
    expect(within(first).queryByRole("textbox", { name: "Edit memo" })).not.toBeInTheDocument();
    expect(within(first).getByText("text")).toBeInTheDocument();
  });

  it("leaves links, tags, task controls and portaled dialogs interactive", () => {
    const [card] = renderCards();
    for (const target of [
      within(card).getByRole("link", { name: "website" }),
      within(card).getByRole("button", { name: "#work" }),
      within(card).getByText("@owner"),
      within(card).getByRole("checkbox"),
      screen.getByText("Dialog preview"),
    ]) {
      fireEvent.doubleClick(target);
      expect(screen.queryByRole("textbox", { name: "Edit memo" })).not.toBeInTheDocument();
    }
  });

  it.each(["other user", "inactive account", "blurred content", "disabled preference"])("does not enter edit mode for %s", (reason) => {
    if (reason === "other user") viewer.name = "users/other";
    if (reason === "inactive account") viewer.state = State.ARCHIVED;
    if (reason === "blurred content") viewer.blur = true;
    if (reason === "disabled preference") localStorage.setItem(DEVICE_PREFERENCES_KEY, JSON.stringify({ doubleClickEdit: false }));
    const [card] = renderCards();
    fireEvent.doubleClick(card);
    expect(screen.queryByRole("textbox", { name: "Edit memo" })).not.toBeInTheDocument();
  });
});
