package api

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/internal/profile"
)

func TestPersonalModuleSideEffectsReturnBeforeUsingDependencies(t *testing.T) {
	service := &APIService{Profile: &profile.Profile{Personal: true}}
	ctx := t.Context()
	// Released memo writes do not query webhook or inbox data and do not queue delivery.
	require.NoError(t, service.DispatchMemoCreatedWebhook(ctx, nil))
	require.NoError(t, service.DispatchMemoUpdatedWebhook(ctx, nil))
	require.NoError(t, service.DispatchMemoDeletedWebhook(ctx, nil))
	require.NoError(t, service.DispatchMemoCommentCreatedWebhook(ctx, nil, nil, 1))
	inbox, err := service.createInboxWithEmailNotification(ctx, nil)
	require.NoError(t, err)
	require.Nil(t, inbox)
}
