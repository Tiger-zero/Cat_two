import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { fireEvent, render, screen, within } from "@testing-library/react";
import React from "react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import MemoView from "@/components/MemoView";
import { useMemoViewContext } from "@/components/MemoView/MemoViewContext";
import { useView, ViewProvider } from "@/contexts/ViewContext";
import { MemoRelation_Type, MemoRelationSchema, MemoSchema } from "@/types/proto/api/memo_service_pb";

const memo = create(MemoSchema, {
  name: "memos/123",
  creator: "users/alice",
  content: "Note body",
  createTime: timestampFromDate(new Date("2026-08-26T16:03:59Z")),
  updateTime: timestampFromDate(new Date("2026-08-27T05:29:37Z")),
});
const origin = "/?filter=tagSearch%3Awork";

vi.mock("@/contexts/AuthContext", () => ({ useAuth: () => ({ currentUser: { name: "users/alice" } }) }));
vi.mock("@/contexts/MentionResolutionContext", () => ({
  useResolvedUser: () => ({ username: "alice", displayName: "Alice", avatarUrl: "" }),
}));
vi.mock("@/components/UserAvatar", () => ({ default: () => <span>avatar</span> }));
vi.mock("@/contexts/NewMemoContext", () => ({ useNewMemo: () => ({ newMemoName: memo.name }) }));
vi.mock("@/components/MemoActionMenu", () => ({
  default: ({ onEdit }: { onEdit: () => void }) => (
    <button type="button" aria-label="memo-actions" onClick={onEdit}>
      Edit memo
    </button>
  ),
}));
vi.mock("@/components/MemoReactionListView", () => ({
  ReactionSelector: ({ trigger }: { trigger: React.ReactElement<{ "aria-label"?: string }> }) =>
    React.cloneElement(trigger, { "aria-label": "add-reaction" }),
}));
vi.mock("@/components/MemoView/components/MemoCommentListView", () => ({ default: () => <aside>Comment preview</aside> }));
vi.mock("@/components/MemoView/components/MemoBody", () => ({
  default: function MemoBody() {
    const { memo, isEditing } = useMemoViewContext();
    return isEditing ? <textarea aria-label="Edit note" defaultValue={memo.content} /> : <p>{memo.content}</p>;
  },
}));
vi.mock("@/utils/i18n", () => ({ useTranslate: () => (key: string) => key }));

function LocationProbe() {
  const location = useLocation();
  return (
    <output data-testid="location">
      {location.pathname}|{JSON.stringify(location.state)}
    </output>
  );
}

function renderCard(note = memo, path = origin) {
  render(
    <MemoryRouter initialEntries={[path]}>
      <MemoView memo={note} showCreator />
      <LocationProbe />
    </MemoryRouter>,
  );
  return screen.getByRole("article");
}

beforeEach(() => localStorage.clear());

describe("memo card header and timestamp", () => {
  it("keeps the creator, new badge and actions above the note, with the Shanghai date and time below", () => {
    const card = renderCard(
      create(MemoSchema, {
        ...memo,
        relations: [
          create(MemoRelationSchema, { type: MemoRelation_Type.COMMENT, memo: { name: "memos/reply" }, relatedMemo: { name: memo.name } }),
        ],
      }),
    );
    const author = within(card).getByText("Alice");
    const badge = within(card).getByText("memo.new-badge");
    const body = within(card).getByText(memo.content);
    const timestamp = within(card).getByText("2026-08-27 00:03");
    expect(author.closest("a, button")).toBeNull();
    for (const element of [author, badge, within(card).getByRole("button", { name: "memo-actions" })]) {
      expect(element.compareDocumentPosition(body) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    }
    const comments = within(card).getByText("Comment preview");
    expect(body.compareDocumentPosition(comments) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(comments.compareDocumentPosition(timestamp) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(timestamp.closest("footer")).toHaveClass("justify-end");
    expect(timestamp.closest("time")).toHaveAttribute("datetime", "2026-08-26T16:03:59.000Z");
    expect(card.querySelectorAll("time")).toHaveLength(1);
  });

  it("keeps the timestamp keyboard-focusable and preserves the filtered origin when opening the detail", () => {
    const card = renderCard();
    const link = within(card).getByRole("link", { name: /2026-08-27 00:03/ });
    expect(link).toHaveAttribute("href", "/memos/123");
    link.focus();
    expect(link).toHaveFocus();
    fireEvent.click(link);
    expect(screen.getByTestId("location")).toHaveTextContent('/memos/123|{"from":"/?filter=tagSearch%3Awork"}');
    expect(within(card).getByText("2026-08-27 00:03").closest("a, button")).toBeNull();
  });

  it("keeps the creation timestamp when sorting by update time and after a later edit", () => {
    function SortByUpdateTime() {
      const { setTimeBasis } = useView();
      return (
        <button type="button" onClick={() => setTimeBasis("update_time")}>
          Sort by update time
        </button>
      );
    }

    const { rerender } = render(<MemoView memo={memo} />, {
      wrapper: ({ children }) => (
        <MemoryRouter>
          <ViewProvider>
            <SortByUpdateTime />
            {children}
          </ViewProvider>
        </MemoryRouter>
      ),
    });
    const timestamp = screen.getByRole("time");
    const created = "2026-08-27 00:03";
    expect(timestamp).toHaveTextContent(created);
    fireEvent.click(screen.getByRole("button", { name: "Sort by update time" }));
    expect(timestamp).toHaveTextContent(created);

    rerender(<MemoView memo={{ ...memo, content: "Edited note", updateTime: timestampFromDate(new Date("2026-08-28T17:15:00Z")) }} />);
    expect(screen.getByText("Edited note")).toBeInTheDocument();
    expect(screen.getByRole("time")).toHaveTextContent(created);
    expect(screen.getByRole("time")).toHaveAttribute("datetime", "2026-08-26T16:03:59.000Z");
  });

  it("renders no timestamp or empty permalink when only the update time is present", () => {
    const card = renderCard({ ...memo, createTime: undefined });
    expect(card.querySelector("time")).toBeNull();
    expect(within(card).queryByRole("link")).toBeNull();
  });

  it("steps aside while the card edits in place", () => {
    const card = renderCard();
    fireEvent.click(within(card).getByRole("button", { name: "memo-actions" }));
    expect(within(card).getByRole("textbox", { name: "Edit note" })).toHaveValue(memo.content);
    expect(card.querySelector("time")).toBeNull();
    expect(within(card).queryByRole("button", { name: "add-reaction" })).toBeNull();
    expect(within(card).queryByRole("button", { name: "memo-actions" })).toBeNull();
    expect(within(card).queryByText("memo.new-badge")).toBeNull();
  });

  it("uses one compact interaction surface for memo header actions", () => {
    renderCard();
    const reaction = screen.getByRole("button", { name: "add-reaction" });
    const actions = screen.getByRole("button", { name: "memo-actions" });
    expect(actions.closest('[data-slot="memo-header-actions"]')).toHaveClass("items-center", "gap-1");
    expect(reaction.parentElement).toHaveClass("sm:group-focus-within:visible", "sm:has-[[data-popup-open]]:visible");
    expect(reaction).toHaveClass("mu-button", "mu-button-ghost", "mu-button-icon", "mu-s-sm");
  });
});
