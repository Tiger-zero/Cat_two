import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { TimestampPopover } from "@/components/MemoEditor/edit/components/TimestampPopover";
import { memoService } from "@/components/MemoEditor/edit/services/memoService";
import { createInitialState, EditorProvider, useEditorContext } from "@/components/MemoEditor/edit/state";
import { type Memo, MemoSchema } from "@/types/proto/api/memo_service_pb";
import { clientSaveOperations as ops } from "./helpers/memo-save-operations";

const clients = vi.hoisted(() => ({ getMemo: vi.fn(), updateMemo: vi.fn() }));
vi.mock("@/connect", () => ({
  attachmentServiceClient: { createAttachment: vi.fn() },
  memoServiceClient: clients,
}));
vi.mock("@/utils/i18n", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/utils/i18n")>()),
  useTranslate: () => (key: string) => key,
}));

const originalCreate = new Date("2020-01-01T10:20:30+08:00");
const originalUpdate = new Date("2020-01-02T11:22:33+08:00");
const customTime = new Date("2024-02-29T04:34:56Z");
let editor: ReturnType<typeof useEditorContext>;

function Probe() {
  editor = useEditorContext();
  return <TimestampPopover />;
}

function renderEditor() {
  const memo = create(MemoSchema, {
    name: "memos/timestamps",
    content: "Original content",
    createTime: timestampFromDate(originalCreate),
    updateTime: timestampFromDate(originalUpdate),
  });
  clients.getMemo.mockResolvedValue(memo);
  render(
    <EditorProvider initialEditorState={{ ...createInitialState(), ...memoService.fromMemo(memo) }}>
      <Probe />
    </EditorProvider>,
  );
  fireEvent.click(screen.getByRole("button"));
  return memo;
}

describe("memo editor timestamps", () => {
  beforeEach(() => {
    clients.updateMemo.mockImplementation(async ({ memo }: { memo: Memo }) => memo);
  });

  it("associates the input label and clears its invalid state after a valid edit", () => {
    renderEditor();
    const input = screen.getByRole("textbox", { name: "common.last-updated-at" });
    expect(screen.getByText("common.last-updated-at")).toHaveAttribute("for", input.id);

    fireEvent.change(input, { target: { value: "invalid" } });
    fireEvent.blur(input);
    expect(input).toHaveAttribute("aria-invalid", "true");

    fireEvent.change(input, { target: { value: "2024-02-29 12:34:56" } });
    expect(input).toHaveAttribute("aria-invalid", "false");
    expect(editor.getState().timestamps.updateTime).toEqual(customTime);
  });

  it("keeps creation time read-only and saves the update time after closing the popover", async () => {
    const memo = renderEditor();
    expect(screen.getByRole("time")).toHaveAttribute("datetime", originalCreate.toISOString());
    expect(screen.queryByRole("textbox", { name: "common.created-at" })).not.toBeInTheDocument();
    const input = screen.getByRole("textbox", { name: "common.last-updated-at" });
    act(() => input.focus());
    fireEvent.change(input, { target: { value: "2024-02-29 12:34:56" } });
    fireEvent.keyDown(input, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("textbox")).not.toBeInTheDocument());

    await memoService.save(editor.getState(), { memoName: memo.name }, ops);

    expect(clients.updateMemo).toHaveBeenCalledOnce();
    const request = clients.updateMemo.mock.calls[0][0];
    expect(request.updateMask.paths).toEqual(["update_time"]);
    expect(request.memo.updateTime).toEqual(timestampFromDate(customTime));
    expect(request.memo.createTime).toBeUndefined();
  });

  it.each([
    "2024-02-30 12:34:56",
    "2024-13-01 12:34:56",
    "2024-02-29 24:34:56",
    "invalid",
  ])("rejects invalid Shanghai time %s without changing the saved timestamp", async (value) => {
    const memo = renderEditor();
    const input = screen.getByRole("textbox", { name: "common.last-updated-at" });
    fireEvent.change(input, { target: { value } });
    fireEvent.blur(input);
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(editor.getState().timestamps.updateTime).toEqual(originalUpdate);
    await memoService.save(editor.getState(), { memoName: memo.name }, ops);
    expect(clients.updateMemo).not.toHaveBeenCalled();
  });

  it("automatically updates the modification time for content edits", async () => {
    const memo = renderEditor();
    const state = { ...editor.getState(), content: "Changed content" };
    await memoService.save(state, { memoName: memo.name }, ops);
    const request = clients.updateMemo.mock.calls[0][0];
    expect(request.updateMask.paths).toEqual(["content", "update_time"]);
    expect(request.memo.updateTime).toBeUndefined();
    expect(request.memo.createTime).toBeUndefined();
  });

  it("keeps the explicitly chosen modification time when content also changes", async () => {
    const memo = renderEditor();
    const input = screen.getByRole("textbox", { name: "common.last-updated-at" });
    fireEvent.change(input, { target: { value: "2024-02-29 12:34:56" } });
    // Saving must see the edit even if the input has not blurred yet.
    await memoService.save({ ...editor.getState(), content: "Changed content" }, { memoName: memo.name }, ops);
    const request = clients.updateMemo.mock.calls[0][0];
    expect(request.updateMask.paths).toEqual(["content", "update_time"]);
    expect(request.memo.updateTime).toEqual(timestampFromDate(customTime));
  });

  it("renders no timestamp before the first save", () => {
    const { container } = render(
      <EditorProvider initialEditorState={createInitialState()}>
        <TimestampPopover />
      </EditorProvider>,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
