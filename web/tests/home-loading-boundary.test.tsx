import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import Home from "@/pages/Home";
import { User_Role } from "@/types/proto/api/user_service_pb";

vi.mock("@/hooks/useMemoSuggestions", () => ({
  useMemoSuggestions: () => [{ id: "tag:work" }],
}));

const state = vi.hoisted(() => ({
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
    renderStickyHeader,
    ...props
  }: {
    renderer: (memo: { name: string }, options: { compact: boolean }) => React.ReactNode;
    renderStickyHeader: () => React.ReactNode;
  } & Record<string, unknown>) => {
    state.listProps = props;
    return (
      <>
        {renderStickyHeader()}
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

vi.mock("@/contexts/MemoFilterContext", () => ({
  useMemoFilterContext: () => ({ filters: [{ factor: "displayTime", value: "2020-06-15" }] }),
}));

vi.mock("@/contexts/NewMemoContext", () => ({
  NewMemoProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
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
    expect(state.listProps).toMatchObject({ contextFilter: 'creator == "users/steven"' });
    expect(state.editorProps?.draft).toStrictEqual({ scope: "home" });
    expect(state.editorProps).toMatchObject({ expandOnFocus: true });
    expect(state.editorProps?.autoFocus).toBeUndefined();
    expect(state.editorProps?.suggestions).toEqual([{ id: "tag:work" }]);
    expect(state.filterOptions).not.toHaveProperty("creatorName");
    expect(state.memoViewProps).toMatchObject({ showPinned: true });
  });

  it("lets a regular user export their own memos", () => {
    render(<Home />);
    fireEvent.click(screen.getByRole("button", { name: "Export memos" }));
    expect(state.exportMemos).toHaveBeenCalledWith("steven", undefined);
  });
});
