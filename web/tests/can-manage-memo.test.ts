import { describe, expect, it } from "vitest";
import { State } from "@/types/proto/api/common_pb";
import { type User, User_Role } from "@/types/proto/api/user_service_pb";
import { canManageAttachment, canManageMemo, isSuperUser } from "@/utils/user";

const user = (name: string, role: User_Role, state = State.NORMAL) => ({ name, role, state }) as User;

describe("canManageMemo", () => {
  const memo = { creator: "users/alice" };

  it("allows the creator", () => {
    expect(canManageMemo(memo, user("users/alice", User_Role.USER))).toBe(true);
  });

  it("reserves note and attachment management for their owner across instance roles", () => {
    const otherAdmin = user("users/root", User_Role.ADMIN);
    expect(isSuperUser(otherAdmin)).toBe(true);
    expect(canManageMemo(memo, otherAdmin)).toBe(false);
    expect(canManageAttachment(memo, otherAdmin)).toBe(false);
    const ownerAdmin = user("users/alice", User_Role.ADMIN);
    expect(canManageMemo(memo, ownerAdmin)).toBe(true);
    expect(canManageAttachment(memo, ownerAdmin)).toBe(true);
  });

  it("denies other users and anonymous viewers", () => {
    expect(canManageMemo(memo, user("users/bob", User_Role.USER))).toBe(false);
    expect(canManageMemo(memo, undefined)).toBe(false);
    expect(canManageMemo(memo, user("users/alice", User_Role.USER, State.ARCHIVED))).toBe(false);
  });
});
