import { ArrowUpLeftFromCircleIcon } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { Link, Navigate, useLocation, useParams } from "react-router-dom";
import MemoCommentSection, { type MemoCommentSectionHandle } from "@/components/MemoCommentSection";
import MemoParentPlaceholder, { type MemoParentStatus } from "@/components/MemoParentPlaceholder";
import MemoView, { type MemoViewHandle } from "@/components/MemoView";
import { computeCommentAmount } from "@/components/MemoView/MemoViewContext";
import { createMemoNavigationState, resolveMemoDetailOrigin } from "@/components/MemoView/navigation";
import { useAppSidebar } from "@/contexts/AppSidebarContext";
import { useAuth } from "@/contexts/AuthContext";
import { useInstance } from "@/contexts/InstanceContext";
import { MentionResolutionProvider } from "@/contexts/MentionResolutionContext";
import useMemoDetailError from "@/hooks/useMemoDetailError";
import { useInfiniteMemoComments, useMemo } from "@/hooks/useMemoQueries";
import { LEGACY_MEMO_COMMENTS_ANCHOR_ID, MEMO_COMMENTS_ANCHOR_ID } from "@/lib/memo-comments";
import { memoNamePrefix } from "@/lib/resource-names";
import { isMemoBlurred } from "@/lib/tag";
import { State } from "@/types/proto/api/common_pb";
import type { Memo } from "@/types/proto/api/memo_service_pb";
import { useTranslate } from "@/utils/i18n";
import { findMemoAnchorTarget } from "@/utils/markdown-manipulation";

const MemoSidebarRegistration = ({
  memo,
  parentMemo,
  parentStatus,
  onParentRetry,
  from,
  commentCount,
  contentHidden,
  onEdit,
  onCommentsOpen,
  onCommentCreate,
  onShareImageOpen,
}: {
  memo: Memo;
  parentMemo?: Memo;
  parentStatus?: MemoParentStatus;
  onParentRetry?: () => void;
  from: string;
  commentCount?: number;
  contentHidden: boolean;
  onEdit: () => void;
  onCommentsOpen: () => void;
  onCommentCreate: () => void;
  onShareImageOpen: () => void;
}) => {
  const { setMemoDetail } = useAppSidebar();

  useEffect(() => {
    setMemoDetail({
      memo,
      parentMemo,
      parentStatus,
      onParentRetry,
      from,
      commentCount,
      contentHidden,
      onEdit,
      onCommentsOpen,
      onCommentCreate,
      onShareImageOpen,
    });
  }, [
    commentCount,
    contentHidden,
    from,
    memo,
    onCommentCreate,
    onCommentsOpen,
    onEdit,
    onShareImageOpen,
    parentMemo,
    parentStatus,
    onParentRetry,
    setMemoDetail,
  ]);

  useEffect(() => () => setMemoDetail(undefined), [setMemoDetail]);

  return null;
};

const MemoDetail = () => {
  const t = useTranslate();
  const { isInitialized: authInitialized, userTagsSetting } = useAuth();
  const { isInitialized: instanceInitialized } = useInstance();
  const [shareImageDialogOpen, setShareImageDialogOpen] = useState(false);
  const params = useParams();
  const location = useLocation();
  const { state: locationState, hash } = location;
  const memoViewRef = useRef<MemoViewHandle>(null);
  const commentSectionRef = useRef<MemoCommentSectionHandle>(null);
  const handleShareImageOpen = useCallback(() => setShareImageDialogOpen(true), []);
  const handleEdit = useCallback(() => memoViewRef.current?.openEditor(), []);

  const memoNameFromParams = params.uid ? `${memoNamePrefix}${params.uid}` : "";
  const { data: memo, error, isLoading, isUnavailable, fetchStatus } = useMemo(memoNameFromParams, { enabled: !!memoNameFromParams });
  const parentPage = resolveMemoDetailOrigin(locationState, { memoArchived: memo?.state === State.ARCHIVED });
  const memoName = memo?.name ?? memoNameFromParams;
  // Keyed by memo so another memo never renders revealed; cleared on leave so returning blurs again.
  const [blurRevealedMemoName, setBlurRevealedMemoName] = useState<string>();
  const showBlurredContent = blurRevealedMemoName === memoName;
  const handleShowBlurredContentChange = useCallback((show: boolean) => setBlurRevealedMemoName(show ? memoName : undefined), [memoName]);
  useEffect(() => setBlurRevealedMemoName(undefined), [memoName]);

  useMemoDetailError({
    error: error as Error | null,
  });

  const {
    data: fetchedParent,
    error: parentError,
    isUnavailable: parentUnavailable,
    refetch: refetchParent,
  } = useMemo(memo?.parent || "", {
    enabled: !!memo?.parent,
  });

  const parentStatus: MemoParentStatus | undefined = memo?.parent
    ? parentUnavailable
      ? "unavailable"
      : parentError
        ? "error"
        : !fetchedParent
          ? "loading"
          : undefined
    : undefined;
  const parentMemo = parentStatus ? undefined : fetchedParent;
  const handleParentRetry = useCallback(() => {
    void refetchParent();
  }, [refetchParent]);

  const {
    data: comments = [],
    fetchNextPage: fetchNextComments,
    hasNextPage: hasNextComments,
    isFetchingNextPage: isFetchingNextComments,
  } = useInfiniteMemoComments(memoName, {
    enabled: !!memo,
  });
  const commentCount = memo ? Math.max(computeCommentAmount(memo), comments.length) : 0;

  const handleCommentsOpen = useCallback(() => {
    commentSectionRef.current?.scrollIntoView();
    window.history.replaceState(window.history.state, "", `${location.pathname}${location.search}#${MEMO_COMMENTS_ANCHOR_ID}`);
  }, [location.pathname, location.search]);

  const handleCommentCreate = useCallback(() => {
    handleCommentsOpen();
    void commentSectionRef.current?.openEditor();
  }, [handleCommentsOpen]);

  // Scroll to the hash target once it's in the DOM. The effect re-runs as the memo loads (footnote
  // anchors) and as comments arrive (comment anchors), since the target may render in either; the
  // ref guards against re-scrolling the same hash on every later comments page-load.
  const scrolledHashRef = useRef("");
  useEffect(() => {
    if (!hash) return;
    const scrollKey = `${memoName}\0${hash}`;
    if (scrolledHashRef.current === scrollKey) return;
    const fragment = decodeURIComponent(hash.slice(1));
    const commentSection = commentSectionRef.current;
    if (commentSection && (fragment === MEMO_COMMENTS_ANCHOR_ID || fragment === LEGACY_MEMO_COMMENTS_ANCHOR_ID)) {
      scrolledHashRef.current = scrollKey;
      commentSection.scrollIntoView();
      return;
    }

    const el = findMemoAnchorTarget(document, memoName, fragment);
    if (!el) return;
    scrolledHashRef.current = scrollKey;
    el.scrollIntoView({ behavior: "smooth", block: "center" });
  }, [hash, memo, memoName, comments]);

  // Keep the query mounted while revalidating a cached denial. Redirecting
  // earlier would abort the request that could confirm restored access.
  if (isUnavailable && fetchStatus === "idle" && !error) return <Navigate to="/404" replace />;

  // Start the permitted requests as soon as routing is unlocked, but do not
  // expose content before tag-blur and instance display settings settle.
  if (isLoading || !memo || !authInitialized || !instanceInitialized) {
    return null;
  }
  const contentHidden = isMemoBlurred(memo, userTagsSetting) && !showBlurredContent;
  const parentBlurred = !!parentMemo && isMemoBlurred(parentMemo, userTagsSetting);
  const mentionResolutionContents = [memo.content, ...comments.map((comment) => comment.content)];
  const userResolutionNames = Array.from(
    new Set([memo, ...comments].flatMap((item) => [item.creator, ...(item.reactions ?? []).map((reaction) => reaction.creator)])),
  );
  return (
    <section className="@container flex min-h-full w-full flex-col items-center pb-8 pt-3 md:pt-6">
      <MentionResolutionProvider contents={mentionResolutionContents} userNames={userResolutionNames}>
        <MemoSidebarRegistration
          memo={memo}
          parentMemo={parentMemo}
          parentStatus={parentStatus}
          onParentRetry={handleParentRetry}
          from={parentPage}
          commentCount={commentCount}
          contentHidden={contentHidden}
          onEdit={handleEdit}
          onCommentsOpen={handleCommentsOpen}
          onCommentCreate={handleCommentCreate}
          onShareImageOpen={handleShareImageOpen}
        />
        <div className="w-full max-w-2xl px-4 sm:px-6">
          <div className="w-full">
            {parentStatus && (
              <div className="mb-2 md:hidden">
                <MemoParentPlaceholder status={parentStatus} onRetry={handleParentRetry} />
              </div>
            )}
            {parentMemo && (
              <div className="w-auto inline-block mb-2 md:hidden">
                <Link
                  className="px-3 py-1 border border-mu-line rounded-mu-lg max-w-xs w-auto mu-body flex flex-row justify-start items-center flex-nowrap text-mu-text-secondary hover:shadow hover:opacity-80"
                  to={`/${parentMemo.name}`}
                  state={createMemoNavigationState(parentPage)}
                  viewTransition
                >
                  <ArrowUpLeftFromCircleIcon className="mu-icon shrink-0 opacity-60 mr-2" />
                  <span className="truncate">{parentBlurred ? t("memo.parent-memo") : parentMemo.content}</span>
                </Link>
              </div>
            )}
            <MemoView
              ref={memoViewRef}
              key={memo.name}
              memo={memo}
              compact={false}
              parentPage={parentPage}
              shareImageDialogOpen={shareImageDialogOpen}
              showBlurredContent={showBlurredContent}
              onShowBlurredContentChange={handleShowBlurredContentChange}
              showCreator
              showPinned
              showSpace
              onShareImageDialogOpenChange={setShareImageDialogOpen}
            />
            <MemoCommentSection
              ref={commentSectionRef}
              memo={memo}
              comments={comments}
              commentCount={commentCount}
              parentPage={parentPage}
              hasMoreComments={hasNextComments}
              isFetchingMoreComments={isFetchingNextComments}
              onLoadMoreComments={fetchNextComments}
            />
          </div>
        </div>
      </MentionResolutionProvider>
    </section>
  );
};

export default MemoDetail;
