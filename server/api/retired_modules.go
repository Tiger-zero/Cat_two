package api

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/usememos/memos/proto/gen/api/apiconnect"
)

const retiredModuleMessage = "this feature is unavailable in Cat_two"

func retiredModuleError() error { return status.Error(codes.Unimplemented, retiredModuleMessage) }

func (s *APIService) personalMode() bool { return s.Profile != nil && s.Profile.Personal }

// isRetiredProcedure is shared by the two transports while the legacy protobuf
// declarations remain available to clients built against older versions.
func isRetiredProcedure(procedure string) bool {
	if strings.HasPrefix(procedure, "/"+apiconnect.SpaceServiceName+"/") ||
		strings.HasPrefix(procedure, "/"+apiconnect.IdentityProviderServiceName+"/") {
		return true
	}
	switch procedure {
	case apiconnect.InstanceServiceGetConfigurationStatusProcedure, apiconnect.InstanceServiceTestInstanceEmailSettingProcedure,
		apiconnect.UserServiceListUserNotificationsProcedure, apiconnect.UserServiceUpdateUserNotificationProcedure,
		apiconnect.UserServiceDeleteUserNotificationProcedure,
		apiconnect.UserServiceListUserWebhooksProcedure, apiconnect.UserServiceCreateUserWebhookProcedure,
		apiconnect.UserServiceUpdateUserWebhookProcedure, apiconnect.UserServiceDeleteUserWebhookProcedure,
		apiconnect.UserServiceGetUserWebhookSigningSecretProcedure,
		apiconnect.UserServiceListLinkedIdentitiesProcedure, apiconnect.UserServiceGetLinkedIdentityProcedure,
		apiconnect.UserServiceCreateLinkedIdentityProcedure, apiconnect.UserServiceDeleteLinkedIdentityProcedure:
		return true
	default:
		return false
	}
}

func rejectRetiredConnectProcedures(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, request connect.AnyRequest) (connect.AnyResponse, error) {
		if isRetiredProcedure(request.Spec().Procedure) {
			return nil, convertGRPCError(retiredModuleError())
		}
		return next(ctx, request)
	}
}
