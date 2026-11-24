package api

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"io"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/usememos/memos/internal/httphandler"
	"github.com/usememos/memos/internal/random"
	"github.com/usememos/memos/internal/ratelimit"
	"github.com/usememos/memos/internal/totp"
	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/store"
)

const (
	twoFactorSetupTTL    = 10 * time.Minute
	twoFactorWindow      = 5 * time.Minute
	twoFactorMaxTries    = 5
	twoFactorBudget      = ratelimit.Scope("totp_account")
	twoFactorIssuer      = "Cat_two"
	twoFactorRequired    = "TOTP_REQUIRED"
	twoFactorInvalid     = "TOTP_INVALID"
	twoFactorUnavailable = "could not access two-factor authentication settings"
)

type twoFactorAction int

const (
	twoFactorSetup twoFactorAction = iota
	twoFactorEnable
	twoFactorDisable
)

type twoFactorStatus struct {
	Enabled bool `json:"enabled"`
}

type twoFactorProvisioning struct {
	Secret     string `json:"secret"`
	OTPAuthURL string `json:"otpauthUrl"`
}

func (s *APIService) registerTwoFactorRoutes(mux *http.ServeMux, authorizer *Authorizer) {
	mux.HandleFunc("GET /internal/auth/totp", requireWebSession(authorizer, s.getTwoFactorStatus))
	mux.HandleFunc("POST /internal/auth/totp/setup", requireWebSession(authorizer, s.changeTwoFactor(twoFactorSetup)))
	mux.HandleFunc("POST /internal/auth/totp/enable", requireWebSession(authorizer, s.changeTwoFactor(twoFactorEnable)))
	mux.HandleFunc("POST /internal/auth/totp/disable", requireWebSession(authorizer, s.changeTwoFactor(twoFactorDisable)))
}

func (s *APIService) getTwoFactorStatus(w http.ResponseWriter, r *http.Request) {
	user, err := s.twoFactorUser(r.Context())
	if err != nil {
		writeHTTPStatusError(w, err)
		return
	}
	state, err := s.Store.GetUserTwoFactor(r.Context(), user.ID)
	if err != nil {
		writeHTTPStatusError(w, status.Error(codes.Internal, twoFactorUnavailable))
		return
	}
	_ = httphandler.JSON(w, http.StatusOK, twoFactorStatus{Enabled: state.ActiveSecret != ""})
}

func (s *APIService) twoFactorUser(ctx context.Context) (*store.User, error) {
	if auth.GetUserClaims(ctx) == nil {
		return nil, status.Error(codes.PermissionDenied, "two-factor settings require a browser sign-in session")
	}
	user, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, twoFactorUnavailable)
	}
	if user == nil {
		return nil, status.Error(codes.Unauthenticated, "sign in again to manage two-factor authentication")
	}
	return user, nil
}

func (s *APIService) changeTwoFactor(action twoFactorAction) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		user, err := s.twoFactorUser(ctx)
		if err != nil {
			writeHTTPStatusError(w, err)
			return
		}
		var body struct {
			Password string `json:"password"`
			Code     string `json:"code"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		if err := decoder.Decode(&body); err != nil || decoder.Decode(new(any)) != io.EOF {
			writeHTTPStatusError(w, status.Error(codes.InvalidArgument, "expected a two-factor settings request"))
			return
		}
		username := accountKey(user.Username)
		if err := s.throttleAndCharge(ratelimit.ScopeSignInAccount, username, 1); err != nil {
			writeHTTPStatusError(w, err)
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.Password)) != nil {
			writeHTTPStatusError(w, status.Error(codes.InvalidArgument, "current password is incorrect"))
			return
		}
		s.refund(ratelimit.ScopeSignInAccount, username, 1)

		revision := auth.GetUserClaims(ctx).SessionRevision
		var response any
		if action == twoFactorSetup {
			response, err = s.prepareTwoFactor(ctx, user, revision)
		} else {
			response, err = s.confirmTwoFactor(ctx, user, revision, action, body.Code)
		}
		if err != nil {
			writeHTTPStatusError(w, err)
			return
		}
		_ = httphandler.JSON(w, http.StatusOK, response)
	}
}

func (s *APIService) prepareTwoFactor(ctx context.Context, user *store.User, revision string) (twoFactorProvisioning, error) {
	secret, err := totp.GenerateSecret()
	if err != nil {
		return twoFactorProvisioning{}, status.Error(codes.Internal, twoFactorUnavailable)
	}
	sealed, err := auth.SealTwoFactorSecret(s.Secret, user.ID, secret)
	if err != nil {
		return twoFactorProvisioning{}, status.Error(codes.Internal, twoFactorUnavailable)
	}
	expires := time.Now().Add(twoFactorSetupTTL).Unix()
	err = s.Store.UpdateUserTwoFactor(ctx, user.ID, &user.PasswordHash, func(state *store.UserTwoFactor) error {
		if state.SessionRevision != revision {
			return store.ErrUserTwoFactorChanged
		}
		if state.ActiveSecret != "" {
			return status.Error(codes.AlreadyExists, "two-factor authentication is already enabled")
		}
		state.PendingSecret = sealed
		state.PendingExpiresTs = expires
		return nil
	})
	if err != nil {
		return twoFactorProvisioning{}, twoFactorStoreError(err)
	}
	return twoFactorProvisioning{Secret: secret, OTPAuthURL: totp.ProvisioningURI(twoFactorIssuer, user.Username, secret)}, nil
}

func (s *APIService) confirmTwoFactor(ctx context.Context, user *store.User, revision string, action twoFactorAction, code string) (twoFactorStatus, error) {
	var verificationErr error
	nextRevision := random.UUID()
	err := s.Store.UpdateUserTwoFactor(ctx, user.ID, &user.PasswordHash, func(state *store.UserTwoFactor) error {
		if state.SessionRevision != revision {
			return store.ErrUserTwoFactorChanged
		}
		now := time.Now()
		key := state.ActiveSecret
		if action == twoFactorEnable {
			if state.ActiveSecret != "" {
				return status.Error(codes.AlreadyExists, "two-factor authentication is already enabled")
			}
			if state.PendingSecret == "" || now.Unix() >= state.PendingExpiresTs {
				return status.Error(codes.InvalidArgument, "two-factor setup expired; start it again")
			}
			key = state.PendingSecret
		} else if key == "" {
			return status.Error(codes.InvalidArgument, "two-factor authentication is not enabled")
		}

		step, err := s.useTwoFactorCode(state, user.ID, key, code, now)
		verificationErr = err
		if err == nil {
			*state = store.UserTwoFactor{SessionRevision: nextRevision}
			if action == twoFactorEnable {
				state.ActiveSecret = key
				state.LastUsedStep = step
			}
		}
		// Commit the attempt budget even if the operation was refused.
		return nil
	})
	if err != nil {
		return twoFactorStatus{}, twoFactorStoreError(err)
	}
	if verificationErr != nil {
		return twoFactorStatus{}, verificationErr
	}
	return twoFactorStatus{Enabled: action == twoFactorEnable}, nil
}

// verifySignInTwoFactor returns the exact session revision accepted by the
// sign-in. If a concurrent enrollment changes it, any tokens issued from this
// result would carry the previous revision and cannot authenticate.
func (s *APIService) verifySignInTwoFactor(ctx context.Context, user *store.User, code string) (string, error) {
	state, err := s.Store.GetUserTwoFactor(ctx, user.ID)
	if err != nil {
		return "", status.Error(codes.Internal, twoFactorUnavailable)
	}
	if state.ActiveSecret == "" {
		return state.SessionRevision, nil
	}
	if code == "" {
		return "", twoFactorError(codes.FailedPrecondition, twoFactorRequired, "enter the code from your authenticator")
	}

	var verificationErr error
	err = s.Store.UpdateUserTwoFactor(ctx, user.ID, &user.PasswordHash, func(current *store.UserTwoFactor) error {
		if current.SessionRevision != state.SessionRevision {
			return store.ErrUserTwoFactorChanged
		}
		step, err := s.useTwoFactorCode(current, user.ID, current.ActiveSecret, code, time.Now())
		verificationErr = err
		if err == nil {
			current.LastUsedStep = step
		}
		return nil
	})
	if err != nil {
		return "", twoFactorStoreError(err)
	}
	return state.SessionRevision, verificationErr
}

// useTwoFactorCode validates a code and updates a persistent attempt budget
// shared by live server processes and sign-in/settings paths.
func (s *APIService) useTwoFactorCode(state *store.UserTwoFactor, userID int32, encrypted, code string, now time.Time) (int64, error) {
	window := time.Unix(state.AttemptWindowTs, 0)
	if !now.Before(window.Add(twoFactorWindow)) {
		state.AttemptWindowTs = now.Unix()
		state.FailedAttempts = 0
		window = time.Unix(state.AttemptWindowTs, 0)
	}
	if state.FailedAttempts >= twoFactorMaxTries {
		return 0, newRateLimitError(twoFactorBudget, ratelimit.Decision{
			Rule:       ratelimit.Rule{Limit: twoFactorMaxTries, Window: twoFactorWindow},
			RetryAfter: window.Add(twoFactorWindow).Sub(now),
		})
	}
	secret, err := auth.OpenTwoFactorSecret(s.Secret, userID, encrypted)
	if err != nil {
		return 0, status.Error(codes.Internal, twoFactorUnavailable)
	}
	step, matched := totp.Match(secret, code, now, state.LastUsedStep)
	if !matched {
		state.FailedAttempts++
		return 0, twoFactorError(codes.InvalidArgument, twoFactorInvalid, "authenticator code is invalid or has already been used")
	}
	state.FailedAttempts = 0
	state.AttemptWindowTs = 0
	return step, nil
}

func twoFactorStoreError(err error) error {
	if stderrors.Is(err, store.ErrUserTwoFactorChanged) {
		return status.Error(codes.FailedPrecondition, "account security settings changed; sign in again")
	}
	if _, ok := status.FromError(err); ok {
		return err
	}
	return status.Error(codes.Internal, twoFactorUnavailable)
}

func twoFactorError(code codes.Code, reason, message string) error {
	st := status.New(code, message)
	detailed, err := st.WithDetails(&errdetails.ErrorInfo{Reason: reason, Domain: errorInfoDomain})
	if err != nil {
		return st.Err()
	}
	return detailed.Err()
}
