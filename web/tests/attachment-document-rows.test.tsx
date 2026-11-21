import { create } from "@bufbuild/protobuf";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AttachmentRows } from "@/components/MemoMetadata/Attachment/AttachmentListView";
import { AttachmentSchema } from "@/types/proto/api/attachment_service_pb";

const mocks = vi.hoisted(() => ({ authenticatedFetch: vi.fn(), download: vi.fn(), showErrorToast: vi.fn() }));

vi.mock("@/connect", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/connect")>()),
  authenticatedFetch: mocks.authenticatedFetch,
}));
vi.mock("@/lib/browser", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/browser")>()),
  downloadFileFromUrl: mocks.download,
}));
vi.mock("@/lib/toast", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/toast")>()),
  showErrorToast: mocks.showErrorToast,
}));

const renderDocument = (fields: { filename: string; type: string; externalLink?: string }) =>
  render(<AttachmentRows audio={[]} docs={[create(AttachmentSchema, { name: "attachments/doc", ...fields })]} />);

const managedDownloadUrl = `${window.location.origin}/file/attachments/doc`;

function clickNativeLink(link: HTMLElement) {
  // Let React handlers run before suppressing jsdom's unsupported navigation.
  document.addEventListener("click", (event) => event.preventDefault(), { once: true });
  fireEvent.click(link);
}

describe("<AttachmentRows> document rows", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("opens a PDF in a new tab and offers its download separately", () => {
    renderDocument({ filename: "report.pdf", type: "application/pdf" });

    const open = screen.getByTitle("Open report.pdf");
    expect(open.getAttribute("target")).toBe("_blank");
    expect(open.hasAttribute("download")).toBe(false);
    const download = screen.getByLabelText("Download report.pdf");
    expect(download.tagName).toBe("A");
    expect(download).toHaveAttribute("download", "report.pdf");
    expect(open).toHaveAttribute("href", `${managedDownloadUrl}/report.pdf`);
    expect(download).toHaveAttribute("href", managedDownloadUrl);
    clickNativeLink(open);
    expect(mocks.authenticatedFetch).not.toHaveBeenCalled();
  });

  it("downloads a file the browser would not show inline", () => {
    renderDocument({ filename: "notes.txt", type: "text/plain" });

    const row = screen.getByTitle("Download notes.txt");
    expect(row.hasAttribute("download")).toBe(true);
    expect(row.hasAttribute("target")).toBe(false);
    expect(screen.queryByTitle(/^Open /)).toBeNull();
  });

  it.each([
    ["report.pdf", "application/pdf"],
    ["项目 计划 100%.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"],
  ])("fetches %s through the authenticated route and saves its original filename", async (filename, type) => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    const createObjectURL = vi.fn<(blob: Blob) => string>().mockReturnValue("blob:document");
    const revokeObjectURL = vi.fn();
    vi.stubGlobal(
      "URL",
      class extends URL {
        static createObjectURL = createObjectURL;
        static revokeObjectURL = revokeObjectURL;
      },
    );
    mocks.authenticatedFetch.mockResolvedValueOnce(new Response("original bytes"));
    renderDocument({ filename, type });

    const download = type === "application/pdf" ? screen.getByLabelText(`Download ${filename}`) : screen.getByTitle(`Download ${filename}`);
    await act(async () => expect(fireEvent.click(download)).toBe(false));

    expect(mocks.download).toHaveBeenCalledWith("blob:document", filename);
    expect(mocks.authenticatedFetch).toHaveBeenCalledExactlyOnceWith(managedDownloadUrl);
    expect(createObjectURL).toHaveBeenCalledOnce();
    expect(await createObjectURL.mock.calls[0][0].text()).toBe("original bytes");
    expect(mocks.showErrorToast).not.toHaveBeenCalled();
    vi.runOnlyPendingTimers();
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:document");
  });

  it("reports an HTTP error without saving the error response as a document", async () => {
    const pending = Promise.withResolvers<Response>();
    mocks.authenticatedFetch.mockReturnValueOnce(pending.promise);
    renderDocument({ filename: "notes.txt", type: "text/plain" });

    const download = screen.getByTitle("Download notes.txt");
    expect(fireEvent.click(download)).toBe(false);
    expect(download).toHaveAttribute("aria-busy", "true");
    fireEvent.click(download);
    expect(mocks.authenticatedFetch).toHaveBeenCalledOnce();

    pending.resolve(new Response("attachment file not found", { status: 404 }));
    await waitFor(() => expect(mocks.showErrorToast).toHaveBeenCalledWith("Failed to download attachment (HTTP 404)."));
    expect(download).not.toHaveAttribute("aria-busy");
    expect(mocks.download).not.toHaveBeenCalled();
  });

  it.each([
    ["旧文稿.doc", "application/msword", "DOC"],
    ["项目 计划.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "DOCX"],
    ["数据.xls", "application/vnd.ms-excel", "XLS"],
    ["数据报告.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "XLSX"],
    ["早期报表.XLSX", "application/octet-stream", "XLSX"],
  ])("uses a concise label and preserves the download filename for %s", (filename, type, label) => {
    renderDocument({ filename, type });

    const row = screen.getByTitle(`Download ${filename}`);
    expect(row).toHaveAttribute("download", filename);
    expect(row).toHaveAttribute("href", managedDownloadUrl);
    expect(screen.getByText(label)).toBeVisible();
    expect(screen.queryByTitle(/^Open /)).toBeNull();
  });

  it("treats a share link as a file on this instance", () => {
    const externalLink = `${managedDownloadUrl}/notes.txt?share_token=abc`;
    renderDocument({
      filename: "notes.txt",
      type: "text/plain",
      externalLink,
    });

    const row = screen.getByTitle("Download notes.txt");
    expect(row).toHaveAttribute("download", "notes.txt");
    expect(row).toHaveAttribute("href", externalLink);
    clickNativeLink(row);
    expect(mocks.authenticatedFetch).not.toHaveBeenCalled();
  });

  it("opens a link to another site in a new tab", () => {
    renderDocument({ filename: "spec.docx", type: "application/msword", externalLink: "https://example.com/spec.docx" });

    const row = screen.getByTitle("Open spec.docx");
    expect(row.getAttribute("href")).toBe("https://example.com/spec.docx");
    expect(row.getAttribute("target")).toBe("_blank");
    expect(row.hasAttribute("download")).toBe(false);
    clickNativeLink(row);
    expect(mocks.authenticatedFetch).not.toHaveBeenCalled();
  });

  it("opens a link that does not parse instead of failing the render", () => {
    renderDocument({ filename: "broken.docx", type: "application/msword", externalLink: "http://[" });

    const row = screen.getByTitle("Open broken.docx");
    expect(row.getAttribute("target")).toBe("_blank");
  });
});
