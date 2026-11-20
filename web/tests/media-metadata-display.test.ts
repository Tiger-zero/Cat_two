import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { describe, expect, it } from "vitest";
import { AttachmentSchema, MediaMetadataSchema, VideoMetadataSchema } from "@/types/proto/api/attachment_service_pb";
import { buildMediaMetadataDisplay, formatExposureTime, formatMediaDuration } from "@/utils/media-metadata";

describe("media metadata display formatting", () => {
  it("uses conventional media duration and shutter-speed notation", () => {
    expect(formatMediaDuration(65.4)).toBe("1:05");
    expect(formatMediaDuration(3661)).toBe("1:01:01");
    expect(formatExposureTime(1 / 120, "en")).toBe("1/120 s");
  });

  it("formats video metadata without requiring photo details", () => {
    const attachment = create(AttachmentSchema, {
      filename: "clip.mp4",
      type: "video/mp4",
      size: 1_048_576n,
      mediaMetadata: create(MediaMetadataSchema, {
        width: 1920,
        height: 1080,
        details: {
          case: "video",
          value: create(VideoMetadataSchema, { durationSeconds: 12.5 }),
        },
      }),
    });

    expect(buildMediaMetadataDisplay([attachment], "en")).toMatchObject({
      file: "MP4 · 1.0 MB",
      dimensions: "1920 × 1080 px",
      duration: "0:13",
      hasSavedMetadata: true,
    });
  });

  it("uses Shanghai time for uploads and keeps the camera's recorded local time", () => {
    const attachment = create(AttachmentSchema, {
      filename: "photo.jpg",
      type: "image/jpeg",
      createTime: timestampFromDate(new Date("2026-11-20T18:00:00Z")),
      mediaMetadata: {
        details: { case: "photo", value: { captureTime: { localDateTime: "2026-11-19T15:30:00", utcOffset: "-08:00" } } },
      },
    });

    expect(buildMediaMetadataDisplay([attachment], "en-US")).toMatchObject({
      uploaded: "Nov 21, 2026, 2:00 AM",
      captured: "Nov 19, 2026, 03:30:00 PM",
      utcOffset: "-08:00",
    });
  });
});
