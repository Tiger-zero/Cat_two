import { create } from "@bufbuild/protobuf";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { memoService } from "@/components/MemoEditor/edit/services/memoService";
import { createInitialState, type EditorState } from "@/components/MemoEditor/edit/state";
import {
  type Memo,
  MemoRelation_MemoRefSchema,
  MemoRelation_Type,
  MemoRelationSchema,
  MemoSchema,
  Visibility,
} from "@/types/proto/api/memo_service_pb";
import { clientSaveOperations as ops } from "./helpers/memo-save-operations";

const clients = vi.hoisted(() => ({
  createMemo: vi.fn(),
  createMemoComment: vi.fn(),
  getMemo: vi.fn(),
  updateMemo: vi.fn(),
}));

vi.mock("@/connect", () => ({
  attachmentServiceClient: {
    createAttachment: vi.fn(),
  },
  memoServiceClient: {
    createMemo: clients.createMemo,
    createMemoComment: clients.createMemoComment,
    getMemo: clients.getMemo,
    updateMemo: clients.updateMemo,
  },
}));

const createRelation = (type: MemoRelation_Type, memoName: string, relatedMemoName: string) =>
  create(MemoRelationSchema, {
    type,
    memo: create(MemoRelation_MemoRefSchema, { name: memoName }),
    relatedMemo: create(MemoRelation_MemoRefSchema, { name: relatedMemoName }),
  });

const createReference = (relatedMemoName: string) =>
  create(MemoRelationSchema, {
    type: MemoRelation_Type.REFERENCE,
    relatedMemo: create(MemoRelation_MemoRefSchema, { name: relatedMemoName }),
  });

const createEditorState = (memo: Memo): EditorState => ({
  ...createInitialState(),
  ...memoService.fromMemo(memo),
});

describe("memo editor relation updates", () => {
  beforeEach(() => {
    clients.createMemo.mockReset().mockImplementation(async ({ memo }: { memo: Memo }) => ({ ...memo, name: "memos/created" }));
    clients.createMemoComment
      .mockReset()
      .mockImplementation(async ({ comment }: { comment: Memo }) => ({ ...comment, name: "memos/comment" }));
    clients.getMemo.mockReset();
    clients.updateMemo.mockReset();
    clients.updateMemo.mockImplementation(async ({ memo }: { memo: Memo }) => memo);
  });

  it("creates a private top-level memo", async () => {
    const state = createInitialState();
    state.content = "Roadmap";

    await memoService.save(state, {}, ops);

    expect(clients.createMemo).toHaveBeenCalledWith({
      memo: expect.objectContaining({ content: "Roadmap", visibility: Visibility.PRIVATE }),
    });
    expect(clients.createMemo.mock.calls[0][0].memo.space).toBeUndefined();
  });

  it("creates a comment on its parent memo", async () => {
    const state = createInitialState();
    state.content = "Reply";
    await memoService.save(state, { parentMemoName: "memos/parent" }, ops);

    expect(clients.createMemoComment).toHaveBeenCalledOnce();
    const request = clients.createMemoComment.mock.calls[0][0];
    expect(request.parent).toBe("memos/parent");
    expect(request.comment.content).toBe("Reply");
  });

  it("edits the body of a legacy note without rewriting its space or visibility", async () => {
    const memo = create(MemoSchema, { name: "memos/legacy", content: "Original", space: "spaces/work", visibility: Visibility.SPACE });
    clients.getMemo.mockResolvedValue(memo);

    await memoService.save({ ...createEditorState(memo), content: "Edited" }, { memoName: memo.name }, ops);

    const { memo: patch, updateMask } = clients.updateMemo.mock.calls[0][0];
    expect(updateMask.paths).toEqual(["content", "update_time"]);
    expect(patch).toMatchObject({ content: "Edited", visibility: Visibility.VISIBILITY_UNSPECIFIED });
    expect(patch.space).toBeUndefined();
  });

  it("sends only mutable references when editing a comment", async () => {
    const commentName = "memos/comment";
    const parentRelation = createRelation(MemoRelation_Type.COMMENT, commentName, "memos/parent");
    const existingReference = createRelation(MemoRelation_Type.REFERENCE, commentName, "memos/existing-reference");
    const incomingReference = createRelation(MemoRelation_Type.REFERENCE, "memos/referencing-comment", commentName);
    const comment = create(MemoSchema, {
      name: commentName,
      content: "Comment",
      relations: [parentRelation, existingReference, incomingReference],
    });
    const addedReference = createReference("memos/added-reference");
    const state = createEditorState(comment);
    state.metadata.relations = [...state.metadata.relations, addedReference];
    clients.getMemo.mockResolvedValue(comment);

    await memoService.save(state, { memoName: commentName }, ops);

    expect(clients.updateMemo).toHaveBeenCalledOnce();
    const request = clients.updateMemo.mock.calls[0][0];
    expect(request.updateMask.paths).toContain("relations");
    expect(request.memo.relations).toEqual([existingReference, addedReference]);
    expect(request.memo.relations.every((relation: { type: MemoRelation_Type }) => relation.type === MemoRelation_Type.REFERENCE)).toBe(
      true,
    );
  });

  it("ignores a new incoming comment when comparing a parent memo's relations", async () => {
    const parentName = "memos/parent";
    const outgoingReference = createRelation(MemoRelation_Type.REFERENCE, parentName, "memos/reference");
    const editorMemo = create(MemoSchema, {
      name: parentName,
      content: "Parent",
      relations: [outgoingReference],
    });
    const freshMemo = create(MemoSchema, {
      name: parentName,
      content: editorMemo.content,
      relations: [outgoingReference, createRelation(MemoRelation_Type.COMMENT, "memos/new-comment", parentName)],
    });
    const state = createEditorState(editorMemo);
    clients.getMemo.mockResolvedValue(freshMemo);

    const result = await memoService.save(state, { memoName: parentName }, ops);

    expect(result).toEqual({ memoName: parentName, hasChanges: false });
    expect(clients.updateMemo).not.toHaveBeenCalled();
  });
});
