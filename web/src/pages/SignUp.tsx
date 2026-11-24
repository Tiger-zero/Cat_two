import { create } from "@bufbuild/protobuf";
import { timestampDate } from "@bufbuild/protobuf/wkt";
import { Button } from "@usememos/ui";
import { useState } from "react";
import { Navigate, useLocation, useSearchParams } from "react-router-dom";
import { setAccessToken } from "@/auth-state";
import AuthNotice from "@/components/AuthNotice";
import AuthPageLayout from "@/components/AuthPageLayout";
import CredentialFields from "@/components/CredentialFields";
import { authServiceClient, userServiceClient } from "@/connect";
import { useAuth } from "@/contexts/AuthContext";
import { useInstance } from "@/contexts/InstanceContext";
import useLoading from "@/hooks/useLoading";
import useNavigateTo from "@/hooks/useNavigateTo";
import { getErrorMessage } from "@/lib/error";
import { ROUTES } from "@/router/routes";
import { UserSchema } from "@/types/proto/api/user_service_pb";
import { AUTH_REASON_PARAM, AUTH_REASON_SIGNUPS_CLOSED, AUTH_REDIRECT_PARAM, getSafeRedirectPath } from "@/utils/auth-redirect";
import { useTranslate } from "@/utils/i18n";

const SignUp = () => {
  const t = useTranslate();
  const navigateTo = useNavigateTo();
  const actionBtnLoadingState = useLoading(false);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [errorMessage, setErrorMessage] = useState("");
  const { initialize: initAuth } = useAuth();
  const { profile, initialize: initInstance } = useInstance();
  const { hash } = useLocation();
  const [searchParams] = useSearchParams();
  const redirectTarget = getSafeRedirectPath(searchParams.get(AUTH_REDIRECT_PARAM));

  const handleFormSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();

    if (username === "" || password === "") {
      return;
    }

    if (actionBtnLoadingState.isLoading) {
      return;
    }

    try {
      actionBtnLoadingState.setLoading();
      setErrorMessage("");
      await userServiceClient.createUser({ user: create(UserSchema, { username, password }) });
      const response = await authServiceClient.signIn({
        credentials: {
          case: "passwordCredentials",
          value: { username, password },
        },
      });
      // Store access token from login response
      if (response.accessToken) {
        setAccessToken(response.accessToken, response.accessTokenExpireTime ? timestampDate(response.accessTokenExpireTime) : undefined);
      }
      // Refresh auth context to load the current user
      await initAuth();
      // Refetch instance profile to update the initialized status
      await initInstance();
      navigateTo(redirectTarget || ROUTES.HOME, { replace: true });
    } catch (error: unknown) {
      console.error(error);
      setErrorMessage(getErrorMessage(error, t("error.sign-up-failed")));
    }
    actionBtnLoadingState.setFinish();
  };

  if (!profile.needsSetup) {
    const params = new URLSearchParams(searchParams);
    params.set(AUTH_REASON_PARAM, AUTH_REASON_SIGNUPS_CLOSED);
    return <Navigate to={{ pathname: ROUTES.AUTH, search: params.toString(), hash }} replace />;
  }

  const submitLabel = t("auth.create-admin-account");

  return (
    <AuthPageLayout title={t("auth.setup-title")} description={t("auth.setup-description")}>
      <form className="flex w-full flex-col gap-4" onSubmit={handleFormSubmit}>
        <CredentialFields
          idPrefix="signup"
          username={username}
          password={password}
          passwordAutoComplete="new-password"
          readOnly={actionBtnLoadingState.isLoading}
          onUsernameChange={(value) => {
            setUsername(value);
            setErrorMessage("");
          }}
          onPasswordChange={(value) => {
            setPassword(value);
            setErrorMessage("");
          }}
        />
        {errorMessage && <AuthNotice tone="error">{errorMessage}</AuthNotice>}
        <Button variant="primary" type="submit" loading={actionBtnLoadingState.isLoading} loadingLabel={submitLabel}>
          {submitLabel}
        </Button>
      </form>
    </AuthPageLayout>
  );
};

export default SignUp;
