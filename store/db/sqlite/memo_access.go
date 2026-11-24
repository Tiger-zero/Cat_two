package sqlite

import "github.com/usememos/memos/store"

// sqliteMemoAccessPredicate authorizes the active author before pagination.
// Historical visibility and placement do not grant access.
func sqliteMemoAccessPredicate(access *store.MemoAccessScope, memoAlias string, args *[]any) string {
	if access.UserID == nil {
		return "1 = 0"
	}
	*args = append(*args, *access.UserID, *access.UserID)
	return memoAlias + ".`creator_id` = ?" +
		" AND EXISTS (SELECT 1 FROM `user` AS `access_user` WHERE `access_user`.`id` = ? AND `access_user`.`row_status` = 'NORMAL')" +
		" AND " + memoAlias + ".`row_status` IN ('NORMAL', 'ARCHIVED')" +
		" AND " + memoAlias + ".`visibility` IN ('PUBLIC', 'PROTECTED', 'PRIVATE', 'SPACE')" +
		" AND " + memoAlias + ".deleted_ts IS NULL"
}
