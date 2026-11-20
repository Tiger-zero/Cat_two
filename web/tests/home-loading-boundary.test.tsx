import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import Home from "@/pages/Home";
import { User_Role } from "@/types/proto/api/user_service_pb";

vi.mock("@/hooks/useMemoSuggestions", () => ({
  useMemoSuggestions: () => [{ id: "tag:work" }],
}));

const state = vi.hoisted(() => ({
  selectedSpaceName: undefined as string | undefined,
  creatorUsername: undefined as string | undefined,
  editorProps: undefined as Record<string, unknown> | undefined,
  listProps: undefined as Record<string, unknown> | undefined,
  memoViewProps: undefined as Record<string, unknown> | undefined,
  filterOptions: undefined as Record<string, unknown> | undefined,
  exportMemos: vi.fn(),
}));

vi.mock("@/components/MemoEditor", () => ({
  MemoEditor: (props: Record<string, unknown>) => {
    state.editorProps = props;
    return <div data-testid="memo-editor" />;
  },
}));

vi.mock("@/components/MemoView", () => ({
  default: (props: Record<string, unknown>) => {
    state.memoViewProps = props;
    return <div data-testid="memo-view" />;
  },
}));

vi.mock("@/components/MemoFeedMenu", () => ({
  default: ({ canExport, onExport }: { canExport: boolean; onExport: () => void }) =>
    canExport ? (
      <button type="button" onClick={onExport}>
        Export memos
      </button>
    ) : null,
}));

vi.mock("@/components/Settings/useExportMemos", () => ({
  useExportMemos: () => ({ exportMemos: state.exportMemos, exporting: false }),
}));

vi.mock("@/components/PagedMemoList", () => ({
  default: ({
    renderer,
    renderLeading,
    renderHeader,
    ...props
  }: {
    renderer: (memo: { name: string }, options: { compact: boolean }) => React.ReactNode;
    renderLeading: (options: { useGrid: boolean }) => React.ReactNode;
    renderHeader: () => React.ReactNode;
  } & Record<string, unknown>) => {
    state.listProps = props;
    return (
      <>
        {renderHeader()}
        {renderLeading({ useGrid: false })}
        {renderer({ name: "memos/1" }, { compact: false })}
      </>
    );
  },
  getMemoKey: (memo: { name: string }) => memo.name,
  EmptyLine: () => null,
}));

vi.mock("@/components/WelcomeGuide", () => ({
  WelcomeGuideDialog: () => null,
  useWelcomeGuide: () => ({ open: false, onOpenChange: () => {} }),
}));

vi.mock("@/contexts/AuthContext", () => ({
  useAuth: () => ({ isUserSettingsInitialized: true }),
}));

vi.mock("@/contexts/GlobalMemoEditorContext", () => ({
  useGlobalMemoEditor: () => ({ claimHomeAutoFocus: () => true }),
}));

vi.mock("@/contexts/MemoFilterContext", () => ({
  useMemoFilterContext: () => ({ filters: [] }),
}));

vi.mock("@/contexts/NewMemoContext", () => ({
  NewMemoProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock("@/contexts/SpaceContext", () => ({
  useSpaceContext: () => ({
    selectedSpaceName: state.selectedSpaceName,
    creatorUsername: state.creatorUsername,
    memoFilter:
      [
        state.selectedSpaceName && `space == "${state.selectedSpaceName}"`,
        state.creatorUsername && `creator == "users/${state.creatorUsername}"`,
      ]
        .filter(Boolean)
        .join(" && ") || undefined,
  }),
}));

vi.mock("@/hooks", () => ({
  useMemoFilters: (options: Record<string, unknown>) => {
    state.filterOptions = options;
    return "";
  },
  useMemoSorting: () => ({ listSort: undefined, orderBy: "create_time desc" }),
}));

vi.mock("@/hooks/useCurrentUser", () => ({
  default: () => ({ name: "users/steven", username: "steven", role: User_Role.USER }),
}));

vi.mock("@/utils/i18n", () => ({
  useTranslate: () => (key: string) => key,
}));

describe("<Home>", () => {
  beforeEach(() => {
    state.selectedSpaceName = undefined;
    state.creatorUsername = undefined;
    state.editorProps = undefined;
    state.listProps = undefined;
    state.memoViewProps = undefined;
    state.filterOptions = undefined;
    state.exportMemos.mockClear();
  });
  it("renders the editor and memo cards synchronously without blank placeholders", () => {
    render(<Home />);

    expect(screen.getByTestId("memo-editor")).toBeInTheDocument();
    expect(screen.getByTestId("memo-view")).toBeInTheDocument();
    expect(state.listProps).toMatchObject({ contextFilter: undefined });
    expect(state.editorProps).toMatchObject({ draft: { scope: "home", space: undefined } });
    expect(state.editorProps?.autoFocus).toEqual(expect.any(Function));
    expect(state.editorProps?.suggestions).toEqual([{ id: "tag:work" }]);
    expect(state.filterOptions).not.toHaveProperty("creatorName");
    expect(state.memoViewProps).toMatchObject({ showCreator: true });
  });

  it("shows another creator's matching memos without offering an editor", () => {
    state.creatorUsername = "alice";
    render(<Home />);
    expect(state.listProps).toMatchObject({ contextFilter: 'creator == "users/alice"' });
    expect(state.memoViewProps).toMatchObject({ showCreator: false });
    expect(screen.queryByTestId("memo-editor")).not.toBeInTheDocument();
  });

  it("filters the feed and sets new memo placement to the selected Space", () => {
    state.selectedSpaceName = "spaces/product";
    render(<Home />);

    expect(state.listProps).toMatchObject({ contextFilter: 'space == "spaces/product"' });
    expect(state.editorProps).toMatchObject({
      draft: { scope: "home", space: "spaces/product" },
    });
  });

  it("lets a regular user export their own memos while browsing another user's feed", () => {
    state.creatorUsername = "alice";
    render(<Home />);
    fireEvent.click(screen.getByRole("button", { name: "Export memos" }));
    expect(state.exportMemos).toHaveBeenCalledWith("steven", undefined);
  });
});
