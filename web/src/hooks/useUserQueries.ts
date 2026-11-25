import { create } from "@bufbuild/protobuf";
import { FieldMaskSchema } from "@bufbuild/protobuf/wkt";
import { queryOptions, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { userServiceClient } from "@/connect";
import useCurrentUser from "@/hooks/useCurrentUser";
import { buildUserSettingName, userNamePrefix } from "@/lib/resource-names";
import { mergeTagCounts } from "@/lib/tag";
import {
  type ListUserStatsRequest,
  ListUserStatsRequestSchema,
  User,
  UserSetting,
  UserSetting_EditorSetting,
  UserSetting_GeneralSetting,
  UserSetting_Key,
  UserSettingSchema,
  UserStats,
} from "@/types/proto/api/user_service_pb";

const BATCH_GET_USERS_LIMIT = 100;
const USER_PROFILE_STALE_TIME = 1000 * 60 * 5;
const OWNER_STATS_STALE_TIME = 1000 * 60 * 2;
type ListUserStatsQuery = Pick<ListUserStatsRequest, "filter">;
// ListUserStats aggregates across every user through the "users/-" wildcard parent.
const ALL_USERS_PARENT = `${userNamePrefix}-`;

// Query keys factory
export const userKeys = {
  all: ["users"] as const,
  details: () => [...userKeys.all, "detail"] as const,
  detail: (name: string) => [...userKeys.details(), name] as const,
  stats: () => [...userKeys.all, "stats"] as const,
  userStats: (name: string, filter?: string) =>
    filter ? ([...userKeys.stats(), name, filter] as const) : ([...userKeys.stats(), name] as const),
  allUserStats: (request: Partial<ListUserStatsQuery>) => [...userKeys.stats(), "all", request] as const,
  currentUser: () => [...userKeys.all, "current"] as const,
  byNames: (names: string[]) => [...userKeys.all, "byNames", ...[...names].sort()] as const,
  byUsernames: (usernames: string[]) => [...userKeys.all, "byUsernames", ...[...usernames].sort()] as const,
};

export const userDetailQueryOptions = (name: string) =>
  queryOptions({
    queryKey: userKeys.detail(name),
    queryFn: () => userServiceClient.getUser({ name }),
    staleTime: USER_PROFILE_STALE_TIME,
  });

export function useUser(name: string, options?: { enabled?: boolean }) {
  return useQuery({
    ...userDetailQueryOptions(name),
    enabled: options?.enabled ?? true,
  });
}

const userStatsQueryOptions = (name?: string, filter?: string) => ({
  queryKey: name ? userKeys.userStats(name, filter) : userKeys.stats(),
  queryFn: async () => {
    if (!name) {
      throw new Error("User name is required");
    }
    return userServiceClient.getUserStats(filter ? { name, filter } : { name, activityByDay: true });
  },
  ...(filter ? {} : { staleTime: OWNER_STATS_STALE_TIME }),
});

export function useUserStats(name?: string, options?: { enabled?: boolean; filter?: string }) {
  const enabled = !!name && (options?.enabled ?? true);
  return useQuery({
    ...userStatsQueryOptions(name, options?.filter),
    enabled,
    subscribed: enabled,
  });
}

const allUserStatsQueryOptions = (request: Partial<ListUserStatsQuery>) => ({
  queryKey: userKeys.allUserStats(request),
  queryFn: async () => {
    const { userStats } = await userServiceClient.listUserStats(
      create(ListUserStatsRequestSchema, { ...request, parent: ALL_USERS_PARENT }),
    );
    return userStats;
  },
});

export function useAllUserStats(request: Partial<ListUserStatsQuery> = {}, options?: { enabled?: boolean }) {
  return useQuery({
    ...allUserStatsQueryOptions(request),
    enabled: options?.enabled ?? true,
  });
}

const selectTagCounts = (data: UserStats | UserStats[]) =>
  Array.isArray(data) ? mergeTagCounts(...data.map((stats) => stats.tagCounts)) : mergeTagCounts(data.tagCounts);

export function useTagCounts(forCurrentUser = false) {
  const currentUser = useCurrentUser();
  const options = forCurrentUser ? userStatsQueryOptions(currentUser?.name) : allUserStatsQueryOptions({});

  return useQuery<UserStats | UserStats[], Error, Record<string, number>>({
    ...options,
    select: selectTagCounts,
    enabled: !forCurrentUser || !!currentUser?.name,
  });
}

export function useUpdateUser() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ user, updateMask }: { user: Partial<User>; updateMask: string[] }) => {
      const updatedUser = await userServiceClient.updateUser({
        user: user as User,
        updateMask: create(FieldMaskSchema, { paths: updateMask }),
      });
      return updatedUser;
    },
    onSuccess: (updatedUser) => {
      queryClient.setQueryData(userKeys.detail(updatedUser.name), updatedUser);
      queryClient.invalidateQueries({ queryKey: userKeys.currentUser() });
    },
  });
}

// Hook to fetch user settings
export function useUserSettings(parent?: string) {
  return useQuery({
    queryKey: [...userKeys.all, "settings", parent],
    queryFn: async () => {
      if (!parent) return { settings: [] };
      const { userSettings: settings } = await userServiceClient.listUserSettings({ parent });
      return { settings };
    },
    enabled: !!parent,
  });
}

// Hook to update user setting
export function useUpdateUserSetting() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ setting, updateMask }: { setting: UserSetting; updateMask: string[] }) => {
      const updatedSetting = await userServiceClient.updateUserSetting({
        userSetting: setting,
        updateMask: create(FieldMaskSchema, { paths: updateMask }),
      });
      return updatedSetting;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [...userKeys.all, "settings"] });
    },
  });
}

// Hook to update user general setting (convenience wrapper)
export function useUpdateUserGeneralSetting(currentUserName?: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ generalSetting, updateMask }: { generalSetting: Partial<UserSetting_GeneralSetting>; updateMask: string[] }) => {
      if (!currentUserName) {
        throw new Error("No current user");
      }

      const settingName = buildUserSettingName(currentUserName, UserSetting_Key.GENERAL);
      const userSetting = create(UserSettingSchema, {
        name: settingName,
        value: {
          case: "generalSetting",
          value: generalSetting as UserSetting_GeneralSetting,
        },
      });

      const updatedSetting = await userServiceClient.updateUserSetting({
        userSetting,
        updateMask: create(FieldMaskSchema, { paths: updateMask }),
      });
      return updatedSetting;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [...userKeys.all, "settings"] });
    },
  });
}

// Hook to update the user's EDITOR setting
export function useUpdateUserEditorSetting(currentUserName?: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (editorSetting: UserSetting_EditorSetting) => {
      if (!currentUserName) {
        throw new Error("No current user");
      }

      const userSetting = create(UserSettingSchema, {
        name: buildUserSettingName(currentUserName, UserSetting_Key.EDITOR),
        value: { case: "editorSetting", value: editorSetting },
      });
      return await userServiceClient.updateUserSetting({
        userSetting,
        updateMask: create(FieldMaskSchema, { paths: ["source_mode"] }),
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [...userKeys.all, "settings"] });
    },
  });
}

// Hook to fetch multiple users by names (returns Map<name, User>)
export function useUsersByNames(names: string[], options?: { enabled?: boolean }) {
  const queryClient = useQueryClient();
  const enabled = (options?.enabled ?? true) && names.length > 0;
  const uniqueNames = Array.from(new Set(names));

  return useQuery({
    queryKey: userKeys.byNames(uniqueNames),
    queryFn: async () => {
      const users = await Promise.all(
        uniqueNames.map(async (name) => {
          try {
            const user = await queryClient.fetchQuery(userDetailQueryOptions(name));
            return { name, user };
          } catch {
            return { name, user: undefined };
          }
        }),
      );

      const userMap = new Map<string, User | undefined>();
      for (const { name, user } of users) {
        userMap.set(name, user);
      }
      return userMap;
    },
    enabled,
    staleTime: USER_PROFILE_STALE_TIME,
  });
}

// Hook to fetch multiple users by usernames (returns Map<username, User>)
export function useUsersByUsernames(usernames: string[], options?: { enabled?: boolean }) {
  const queryClient = useQueryClient();
  const enabled = (options?.enabled ?? true) && usernames.length > 0;
  const uniqueUsernames = Array.from(new Set(usernames));

  return useQuery({
    queryKey: userKeys.byUsernames(uniqueUsernames),
    queryFn: async () => {
      const usersByUsername = new Map<string, User>();
      const missingUsernames: string[] = [];
      for (const username of uniqueUsernames) {
        const detailKey = userKeys.detail(`${userNamePrefix}${username}`);
        const cachedUser = queryClient.getQueryData<User>(detailKey);
        const cachedState = queryClient.getQueryState(detailKey);
        const cacheIsFresh = cachedState ? Date.now() - cachedState.dataUpdatedAt < USER_PROFILE_STALE_TIME : false;
        if (cachedUser && cacheIsFresh) {
          usersByUsername.set(username, cachedUser);
        } else {
          missingUsernames.push(username);
        }
      }

      const batches = [];
      for (let i = 0; i < missingUsernames.length; i += BATCH_GET_USERS_LIMIT) {
        batches.push(missingUsernames.slice(i, i + BATCH_GET_USERS_LIMIT));
      }

      const responses = await Promise.all(
        batches.map((batch) => userServiceClient.batchGetUsers({ names: batch.map((username) => `${userNamePrefix}${username}`) })),
      );
      const users = responses.flatMap((response) => response.users);
      for (const user of users) {
        usersByUsername.set(user.username, user);
        queryClient.setQueryData(userKeys.detail(user.name), user);
      }

      const userMap = new Map<string, User | undefined>();
      for (const username of uniqueUsernames) {
        userMap.set(username, usersByUsername.get(username));
      }
      return userMap;
    },
    enabled,
    staleTime: USER_PROFILE_STALE_TIME,
  });
}
