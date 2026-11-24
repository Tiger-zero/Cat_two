import { create } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MemoEditor } from "@/components/MemoEditor/MemoEditor";
import { memoKeys } from "@/hooks/useMemoQueries";
import { toast } from "@/lib/toast";
import { ListMemosResponseSchema, type Memo, MemoSchema } from "@/types/proto/api/memo_service_pb";

const state = vi.hoisted(() => ({ currentUser: undefined as { name: string; role?: string } | undefined }));
const api = vi.hoisted(() => ({ getMemo: vi.fn(), updateMemo: vi.fn() }));
const recordEditing = vi.hoisted(() => vi.fn());

vi.mock("@/connect", () => ({ memoServiceClient: api }));
vi.mock("@/lib/memo-editing-activity", () => ({ recordMemoEditingActivity: recordEditing }));
vi.mock("@/hooks/useCurrentUser", () => ({ default: () => state.currentUser }));
vi.mock("@/contexts/AuthContext", () => ({ useAuth: () => ({ userTagsSetting: undefined }) }));
vi.mock("@/contexts/ViewContext", () => ({ useLinkPreviewEnabled: () => false }));
vi.mock("@/contexts/MemoFilterContext", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/contexts/MemoFilterContext")>()),
  useMemoFilterContext: () => ({ getFiltersByFactor: () => [], addFilter: vi.fn(), removeFilter: vi.fn() }),
}));
vi.mock("@/contexts/MentionResolutionContext", () => ({ useResolvedMentionUsernames: () => new Set<string>() }));
vi.mock("@/hooks/useNavigateTo", () => ({ default: () => vi.fn() }));

const setup = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const renderMemo = (name: string) =>
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <MemoEditor name={name} />
        </MemoryRouter>
      </QueryClientProvider>,
    );
  return { client, renderMemo };
};

const memo = (content: string): Memo => create(MemoSchema, { name: "memos/one", creator: "users/owner", content });

describe("MemoEditor in view mode", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    state.currentUser = undefined;
  });

  it("shows the memo a list already holds, without fetching it", () => {
    const { client, renderMemo } = setup();
    client.setQueryData(memoKeys.list({}), create(ListMemosResponseSchema, { memos: [memo("# Title\n\nSome **bold** text")] }));

    const { container } = renderMemo("memos/one");

    expect(container.querySelector(".memo-content-root")).toHaveAttribute("aria-readonly", "true");
    expect(container.querySelector("h1")).toHaveTextContent("Title");
    expect(container.querySelector("[data-memo-name]")).toHaveAttribute("data-memo-name", "memos/one");
    expect(api.getMemo).not.toHaveBeenCalled();
    expect(recordEditing).not.toHaveBeenCalled();
  });

  it("fetches a memo no query holds", async () => {
    const { renderMemo } = setup();
    api.getMemo.mockResolvedValue(memo("Fetched"));

    const { container } = renderMemo("memos/one");

    await waitFor(() => expect(container.querySelector(".memo-content-root")).toHaveTextContent("Fetched"));
    expect(api.getMemo).toHaveBeenCalledOnce();
  });

  it("lets the owner tick a task and saves it through the mutation, so the list follows", async () => {
    const { client, renderMemo } = setup();
    state.currentUser = { name: "users/owner" };
    client.setQueryData(memoKeys.list({}), create(ListMemosResponseSchema, { memos: [memo("- [ ] First\n- [x] Second")] }));
    api.updateMemo.mockImplementation(async ({ memo: update }: { memo: Memo }) => ({ ...memo(update.content), content: update.content }));

    const { container } = renderMemo("memos/one");
    const checkbox = container.querySelector<HTMLInputElement>("input.mbe-checkbox");
    expect(checkbox).not.toBeDisabled();
    await act(async () => {
      fireEvent.mouseDown(checkbox!);
    });

    await waitFor(() => expect(api.updateMemo).toHaveBeenCalledOnce());
    expect(recordEditing).toHaveBeenCalledOnce();
    const [{ memo: sent, updateMask }] = api.updateMemo.mock.calls[0];
    expect(sent.content).toBe("- [x] First\n- [x] Second");
    expect(updateMask.paths).toEqual(["content", "update_time"]);
    await waitFor(() =>
      expect(client.getQueryData<{ memos: Memo[] }>(memoKeys.list({}))?.memos[0].content).toBe("- [x] First\n- [x] Second"),
    );
  });

  it.each([
    false,
    true,
  ])("uses only the previous locally committed revision when several tasks are clicked (external edit: %s)", async (externalEdit) => {
    const { client, renderMemo } = setup();
    state.currentUser = { name: "users/owner" };
    const before = { ...memo("- [ ] First\n- [ ] Second"), revision: "42:1" };
    const savedFirst = { ...before, revision: "42:2", content: "- [x] First\n- [ ] Second" };
    const savedBoth = { ...before, revision: "42:3", content: "- [x] First\n- [x] Second" };
    const fromOtherClient = { ...savedFirst, revision: "42:3", content: `${savedFirst.content}\n\nEdit from another device` };
    let stored = before;
    let replyFirst!: () => void;
    const showError = vi.spyOn(toast, "add");
    client.setQueryData(memoKeys.list({}), create(ListMemosResponseSchema, { memos: [before] }));
    api.updateMemo
      .mockImplementationOnce(
        () =>
          new Promise<Memo>((resolve) => {
            replyFirst = () => {
              stored = externalEdit ? fromOtherClient : savedFirst;
              resolve(savedFirst);
            };
          }),
      )
      .mockImplementationOnce(async ({ expectedRevision }: { expectedRevision: string }) => {
        if (expectedRevision !== stored.revision) throw new ConnectError("memo revision changed", Code.Aborted);
        stored = savedBoth;
        return stored;
      });

    const { container } = renderMemo(before.name);
    const clickTask = (index: number) => {
      fireEvent.mouseDown(container.querySelectorAll<HTMLInputElement>("input.mbe-checkbox")[index]);
    };
    act(() => clickTask(0));
    await waitFor(() => expect(api.updateMemo).toHaveBeenCalledOnce());
    act(() => clickTask(1));
    await waitFor(() => expect(client.isMutating()).toBe(2));
    expect(api.updateMemo).toHaveBeenCalledOnce();
    await act(async () => replyFirst());
    await waitFor(() => expect(client.isMutating()).toBe(0));

    expect(api.updateMemo).toHaveBeenNthCalledWith(1, expect.objectContaining({ expectedRevision: before.revision }));
    expect(api.updateMemo).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({ expectedRevision: savedFirst.revision, memo: expect.objectContaining({ content: savedBoth.content }) }),
    );
    expect(stored).toEqual(externalEdit ? fromOtherClient : savedBoth);
    if (externalEdit) expect(showError).toHaveBeenCalledWith(expect.objectContaining({ type: "danger" }));
    else expect(showError).not.toHaveBeenCalled();
  });

  it("keeps tasks locked for a reader", () => {
    const { client, renderMemo } = setup();
    state.currentUser = { name: "users/reader" };
    client.setQueryData(memoKeys.list({}), create(ListMemosResponseSchema, { memos: [memo("- [ ] First")] }));

    const { container } = renderMemo("memos/one");

    expect(container.querySelector("input.mbe-checkbox")).toBeDisabled();
    expect(recordEditing).not.toHaveBeenCalled();
  });
});
