package api

// PublicMethods defines API endpoints that don't require authentication.
// All other endpoints require a valid session or access token.
//
// This is the SINGLE SOURCE OF TRUTH for public endpoints.
// Both Connect interceptor and gRPC-Gateway interceptor use this map.
//
// Format: Full gRPC procedure path as returned by req.Spec().Procedure (Connect)
// or info.FullMethod (gRPC interceptor).
var PublicMethods = map[string]struct{}{
	// Auth Service - login/token endpoints must be accessible without auth
	"/memos.api.AuthService/SignIn":       {},
	"/memos.api.AuthService/RefreshToken": {}, // Token refresh uses cookie, must be accessible when access token expired

	// Instance Service - needed before login to show instance info
	"/memos.api.InstanceService/GetInstanceProfile":       {},
	"/memos.api.InstanceService/GetInstanceSetting":       {},
	"/memos.api.InstanceService/BatchGetInstanceSettings": {},

	// User Service - account registration and profiles
	"/memos.api.UserService/CreateUser":    {}, // Registration policy is enforced in UserService
	"/memos.api.UserService/GetUser":       {},
	"/memos.api.UserService/BatchGetUsers": {},

	// Identity Provider Service - SSO buttons on login page
	"/memos.api.IdentityProviderService/ListIdentityProviders": {},

	// Link previews do not read stored notes.
	"/memos.api.MemoService/GetLinkMetadata":      {},
	"/memos.api.MemoService/BatchGetLinkMetadata": {},

	// Disabled share links return the same not-found result for every caller.
	"/memos.api.MemoService/GetSharedMemo": {},
}

// IsPublicMethod checks if a procedure path is public (no authentication required).
// Returns true for public methods, false for protected methods.
func IsPublicMethod(procedure string) bool {
	_, ok := PublicMethods[procedure]
	return ok
}

// AuthBootstrapMethods is the subset of PublicMethods that stays reachable by
// anonymous callers even when the instance access mode is PRIVATE.
//
// It allows sign-in, permitted account registration, and consistent rejection
// of retired share links. Every entry here
// MUST also exist in PublicMethods.
var AuthBootstrapMethods = map[string]struct{}{
	// Auth Service - sign-in and token refresh.
	"/memos.api.AuthService/SignIn":       {},
	"/memos.api.AuthService/RefreshToken": {},

	// Instance Service - needed to render the sign-in page (branding, auth options).
	"/memos.api.InstanceService/GetInstanceProfile":       {},
	"/memos.api.InstanceService/GetInstanceSetting":       {},
	"/memos.api.InstanceService/BatchGetInstanceSettings": {},

	// Identity Provider Service - SSO buttons on the sign-in page.
	"/memos.api.IdentityProviderService/ListIdentityProviders": {},

	// User Service - CreateUser applies registration and password-auth settings.
	"/memos.api.UserService/CreateUser": {},

	// Retired share links return not found on both instance access modes.
	"/memos.api.MemoService/GetSharedMemo": {},
}

// IsAuthBootstrapMethod reports whether an anonymous request to procedure is one
// of the fixed endpoints allowed while the instance is private.
func IsAuthBootstrapMethod(procedure string) bool {
	_, ok := AuthBootstrapMethods[procedure]
	return ok
}
