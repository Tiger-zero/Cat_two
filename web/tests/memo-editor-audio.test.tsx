import { create } from "@bufbuild/protobuf";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeAll, describe, expect, it, vi } from "vitest";
import { MemoEditor, preloadMemoEditing } from "@/components/MemoEditor";
import { uploadService } from "@/components/MemoEditor/edit/services";
import { memoKeys } from "@/hooks/useMemoQueries";
import { AttachmentSchema } from "@/types/proto/api/attachment_service_pb";
import { MemoSchema, Visibility } from "@/types/proto/api/memo_service_pb";

vi.mock("@/hooks/useUserQueries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/hooks/useUserQueries")>()),
  useTagCounts: () => ({ data: {} }),
  useUsersByUsernames: () => ({ data: undefined }),
  useUpdateUserEditorSetting: () => ({ mutate: vi.fn() }),
}));
vi.mock("@/hooks/useCurrentUser", () => ({ default: () => ({ name: "users/1" }) }));
vi.mock("@/contexts/AuthContext", () => ({ useAuth: () => ({ userGeneralSetting: {} }) }));
vi.mock("@/components/MemoEditor/edit/hooks/useAudioWaveform", () => ({ useAudioWaveform: () => [] }));
vi.mock("@/contexts/NewMemoContext", () => ({ useNewMemo: () => ({ markNewMemo: vi.fn() }) }));
const clients = vi.hoisted(() => ({ getMemo: vi.fn(), updateMemo: vi.fn(), listMemoAttachments: vi.fn() }));
vi.mock("@/connect", () => ({ attachmentServiceClient: {}, memoServiceClient: clients, userServiceClient: {} }));
vi.stubGlobal("matchMedia", () => ({ matches: false, addEventListener() {}, removeEventListener() {} }));
vi.stubGlobal(
  "ResizeObserver",
  class {
    observe() {}
    unobserve() {}
    disconnect() {}
  },
);

// The editing session loads lazily; loaded once, it renders without suspending.
beforeAll(() => preloadMemoEditing());

class TestMediaRecorder extends EventTarget {
  static isTypeSupported = () => true;
  state: RecordingState = "inactive";

  start() {
    this.state = "recording";
  }

  stop() {
    this.state = "inactive";
    this.dispatchEvent(Object.assign(new Event("dataavailable"), { data: new Blob(["audio"]) }));
    this.dispatchEvent(new Event("stop"));
  }
}

describe("memo audio attachments", () => {
  it("records, previews, and saves audio alongside an existing recording", async () => {
    const stopTrack = vi.fn();
    const getUserMedia = vi.fn().mockResolvedValue({ getTracks: () => [{ stop: stopTrack }] });
    Object.defineProperty(navigator, "mediaDevices", { configurable: true, value: { getUserMedia } });
    vi.stubGlobal("MediaRecorder", TestMediaRecorder);
    const createPreview = vi.fn().mockReturnValue("blob:new-audio");
    vi.stubGlobal(
      "URL",
      class extends URL {
        static createObjectURL = createPreview;
        static revokeObjectURL = vi.fn();
      },
    );
    const play = vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue(undefined);

    const attachment = create(AttachmentSchema, {
      name: "attachments/voice",
      filename: "voice-note-20261120-100000.webm",
      type: "audio/webm",
    });
    const recording = create(AttachmentSchema, { name: "attachments/new-voice", filename: "new-voice.webm", type: "audio/webm" });
    const memo = create(MemoSchema, {
      name: "memos/meeting",
      creator: "users/1",
      content: "Original meeting note",
      visibility: Visibility.PRIVATE,
      attachments: [attachment],
    });
    clients.getMemo.mockResolvedValue(memo);
    clients.updateMemo.mockImplementation(async ({ memo: update }) => create(MemoSchema, { ...memo, ...update }));
    const upload = vi.spyOn(uploadService, "uploadFile").mockResolvedValue(recording);
    const client = new QueryClient();
    const onConfirm = vi.fn();
    client.setQueryData(memoKeys.detail(memo.name), memo);
    const { container } = render(
      <QueryClientProvider client={client}>
        <MemoEditor mode="edit" name={memo.name} onConfirm={onConfirm} />
      </QueryClientProvider>,
    );

    await waitFor(() => expect(container.querySelector(".memo-editor-root")?.textContent).toBe(memo.content));
    fireEvent.click(screen.getByRole("button", { name: "Add" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "Record audio" }));
    const stopRecording = await screen.findByRole("button", { name: "Stop recording" });
    expect(getUserMedia).toHaveBeenCalledWith({ audio: true });
    expect(screen.getByRole("button", { name: "Update" })).toHaveAttribute("aria-disabled", "true");
    fireEvent.click(stopRecording);

    await waitFor(() => expect(screen.getAllByRole("button", { name: "Play audio recording" })).toHaveLength(2));
    expect(stopTrack).toHaveBeenCalledOnce();
    expect(createPreview).toHaveBeenCalledWith(expect.objectContaining({ type: "audio/webm;codecs=opus" }));
    fireEvent.click(screen.getAllByRole("button", { name: "Play audio recording" })[1]);
    expect(play).toHaveBeenCalledOnce();
    expect(container.querySelector('audio[src="blob:new-audio"]')).toBeInTheDocument();
    expect(upload).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Update" }));
    await waitFor(() => expect(onConfirm).toHaveBeenCalledWith(memo.name));
    expect(clients.updateMemo).toHaveBeenCalledOnce();
    expect(upload).toHaveBeenCalledOnce();
    const request = clients.updateMemo.mock.calls[0][0];
    expect(request.memo.attachments.map(({ name }: { name: string }) => name)).toEqual([attachment.name, recording.name]);
    expect(request.updateMask.paths).toContain("attachments");
    expect(request.updateMask.paths).not.toContain("content");
  });
});
