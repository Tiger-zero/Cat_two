import { State } from "@/types/proto/api/common_pb";
import { type User, User_Role } from "@/types/proto/api/user_service_pb";

export const isSuperUser = (user: User | undefined) => {
  return user && user.role === User_Role.ADMIN;
};

/** Whether the user is the memo's author and their account is still active. */
export const canManageMemo = (memo: { creator: string }, user: User | undefined): boolean => {
  return !!user && user.state !== State.ARCHIVED && memo.creator === user.name;
};

/** Whether the user is the attachment's active uploader. */
export const canManageAttachment = (attachment: { creator: string }, user: User | undefined): boolean => canManageMemo(attachment, user);
