import { create } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { memoKeys } from "@/hooks/useMemoQueries";
import MemoDetail from "@/pages/MemoDetail";
import { RequireAuthRoute } from "@/router/guards";
import { type Memo, MemoSchema } from "@/types/proto/api/memo_service_pb";

const state = vi.hoisted(() => ({
  currentUser: undefined as { name: string } | undefined,
  getMemo: vi.fn(),
  listMemoComments: vi.fn(),
  setMemoDetail: vi.fn(),
  toastError: vi.fn(),
}));
vi.mock("@/connect", () => ({ memoServiceClient: state }));
vi.mock("@/contexts/AuthContext", () => ({ useAuth: () => ({ currentUser: state.currentUser, isInitialized: true }) }));
vi.mock("@/contexts/InstanceContext", () => ({ useInstance: () => ({ isInitialized: true }) }));
vi.mock("@/contexts/AppSidebarContext", () => ({ useAppSidebar: () => ({ setMemoDetail: state.setMemoDetail }) }));
vi.mock("@/components/MemoView", () => ({ default: ({ memo }: { memo: Memo }) => <article>{memo.content}</article> }));
vi.mock("@/components/MemoView/MemoViewContext", () => ({ computeCommentAmount: () => 0 }));
vi.mock("@/components/MemoCommentSection", () => ({ default: () => null }));
vi.mock("@/contexts/MentionResolutionContext", () => ({
  MentionResolutionProvider: ({ children }: { children: ReactNode }) => children,
}));
vi.mock("@/utils/i18n", () => ({ useTranslate: () => (key: string) => key }));
vi.mock("@/lib/toast", () => ({ toast: { add: state.toastError, close: vi.fn() }, showErrorToast: state.toastError }));

function SignInDestination() {
  const { search } = useLocation();
  return <div>Sign in to continue to {new URLSearchParams(search).get("redirect")}</div>;
}

function renderDetail(client: QueryClient, name = "memos/restored") {
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[`/${name}`]}>
        <Routes>
          <Route element={<RequireAuthRoute />}>
            <Route path="/memos/:uid" element={<MemoDetail />} />
          </Route>
          <Route path="/auth" element={<SignInDestination />} />
          <Route path="/404" element={<div>not found</div>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}
const createClient = () => new QueryClient({ defaultOptions: { queries: { retry: false, retryDelay: 0 } } });

describe("memo detail access recovery", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    state.currentUser = { name: "users/alice" };
    state.listMemoComments.mockResolvedValue({ comments: [], nextPageToken: "" });
  });

  it.each(["fresh", "stale"])("revalidates a %s cached denial before showing a restored memo", async (freshness) => {
    const client = createClient();
    const name = "memos/restored";
    client.setQueryData(memoKeys.detail(name), null, { updatedAt: freshness === "fresh" ? Date.now() : 1 });
    let resolve!: (memo: Memo) => void;
    let signal!: AbortSignal;
    state.getMemo.mockImplementation((_, options) => {
      signal = options.signal;
      return new Promise<Memo>((done) => {
        resolve = done;
      });
    });
    renderDetail(client);
    await waitFor(() => expect(state.getMemo).toHaveBeenCalledOnce());
    expect(screen.queryByText("not found")).not.toBeInTheDocument();
    expect(signal.aborted).toBe(false);
    await act(async () => resolve(create(MemoSchema, { name, content: "Restored memo" })));
    expect(await screen.findByText("Restored memo")).toBeInTheDocument();
    expect(screen.queryByText("not found")).not.toBeInTheDocument();
    client.clear();
  });

  it.each([Code.PermissionDenied, Code.NotFound])("redirects only after revalidation confirms denial (%s)", async (code) => {
    const client = createClient();
    client.setQueryData(memoKeys.detail("memos/restored"), null, { updatedAt: 1 });
    let reject!: (error: Error) => void;
    state.getMemo.mockImplementation(
      () =>
        new Promise((_, fail) => {
          reject = fail;
        }),
    );
    renderDetail(client);
    await waitFor(() => expect(state.getMemo).toHaveBeenCalledOnce());
    expect(screen.queryByText("not found")).not.toBeInTheDocument();
    await act(async () => reject(new ConnectError("denied", code)));
    expect(await screen.findByText("not found")).toBeInTheDocument();
    client.clear();
  });

  it("does not treat a failed revalidation as a renewed denial", async () => {
    const client = createClient();
    client.setQueryData(memoKeys.detail("memos/restored"), null, { updatedAt: 1 });
    state.getMemo.mockRejectedValue(new ConnectError("offline", Code.Unavailable));
    renderDetail(client);
    await waitFor(() => expect(state.toastError).toHaveBeenCalled());
    expect(screen.queryByText("not found")).not.toBeInTheDocument();
    client.clear();
  });

  it("preserves the direct link for a guest without requesting memo data", async () => {
    state.currentUser = undefined;
    const client = createClient();
    renderDetail(client);
    expect(await screen.findByText("Sign in to continue to /memos/restored")).toBeInTheDocument();
    expect(state.getMemo).not.toHaveBeenCalled();
    expect(state.listMemoComments).not.toHaveBeenCalled();
    client.clear();
  });

  it("lets a signed-in user retry after a parent returns 401", async () => {
    const client = createClient();
    const comment = create(MemoSchema, { name: "memos/comment", content: "My comment", parent: "memos/private-parent" });
    state.getMemo.mockImplementation(({ name }) =>
      name === comment.name ? Promise.resolve(comment) : Promise.reject(new ConnectError("authentication required", Code.Unauthenticated)),
    );
    renderDetail(client, comment.name);
    expect(await screen.findByText("memo.parent-load-error")).toBeInTheDocument();
    expect(screen.getByText("My comment")).toBeInTheDocument();
    expect(screen.queryByText("not found")).not.toBeInTheDocument();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "search.retry" })).toBeInTheDocument();
    expect(state.setMemoDetail).toHaveBeenLastCalledWith(
      expect.objectContaining({
        parentStatus: "error",
        parentMemo: undefined,
      }),
    );
    client.clear();
  });

  it("lets a signed-in user retry a transient parent error", async () => {
    const client = createClient();
    const comment = create(MemoSchema, { name: "memos/comment", content: "My comment", parent: "memos/parent" });
    let parentAvailable = false;
    state.getMemo.mockImplementation(({ name }) =>
      name === comment.name
        ? Promise.resolve(comment)
        : parentAvailable
          ? Promise.resolve(create(MemoSchema, { name, content: "Parent memo" }))
          : Promise.reject(new ConnectError("offline", Code.Unavailable)),
    );
    renderDetail(client, comment.name);
    const retry = await screen.findByRole("button", { name: "search.retry" });
    parentAvailable = true;
    fireEvent.click(retry);
    expect(await screen.findByRole("link", { name: "Parent memo" })).toBeInTheDocument();
    expect(screen.getByText("My comment")).toBeInTheDocument();
    client.clear();
  });
});
