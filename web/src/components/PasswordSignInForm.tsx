import { timestampDate } from "@bufbuild/protobuf/wkt";
import { Code } from "@connectrpc/connect";
import { Button } from "@usememos/ui";
import { useState } from "react";
import { setAccessToken } from "@/auth-state";
import AuthNotice from "@/components/AuthNotice";
import ChallengeWidget, { CHALLENGE_TOKEN_HEADER } from "@/components/ChallengeWidget";
import CredentialFields from "@/components/CredentialFields";
import { TwoFactorCodeField, TwoFactorRecovery } from "@/components/TwoFactorFields";
import { authServiceClient } from "@/connect";
import { useAuth } from "@/contexts/AuthContext";
import useLoading from "@/hooks/useLoading";
import useNavigateTo from "@/hooks/useNavigateTo";
import { ERROR_REASON_CHALLENGE_REQUIRED, getErrorMessage, hasConnectCode, hasErrorReason } from "@/lib/error";
import { ROUTES } from "@/router/routes";
import { useTranslate } from "@/utils/i18n";

interface PasswordSignInFormProps {
  redirectPath?: string;
}

interface SignInError {
  message: string;
  field: "credentials" | "code" | "form";
}

function PasswordSignInForm({ redirectPath }: PasswordSignInFormProps) {
  const t = useTranslate();
  const navigateTo = useNavigateTo();
  const { initialize } = useAuth();
  const actionBtnLoadingState = useLoading(false);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [needsCode, setNeedsCode] = useState(false);
  const [code, setCode] = useState("");
  const [error, setError] = useState<SignInError>();
  const [challengeToken, setChallengeToken] = useState<string | null>(null);
  const [challengeResetKey, setChallengeResetKey] = useState(0);

  const handleFormSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();

    if (username === "" || password === "" || actionBtnLoadingState.isLoading || (needsCode && code.length !== 6)) {
      return;
    }

    try {
      actionBtnLoadingState.setLoading();
      setError(undefined);
      const response = await authServiceClient.signIn(
        {
          credentials: {
            case: "passwordCredentials",
            value: { username, password, ...(needsCode ? { otpCode: code } : {}) },
          },
        },
        challengeToken ? { headers: { [CHALLENGE_TOKEN_HEADER]: challengeToken } } : undefined,
      );
      // Store access token from login response
      if (response.accessToken) {
        setAccessToken(response.accessToken, response.accessTokenExpireTime ? timestampDate(response.accessTokenExpireTime) : undefined);
      }
      await initialize();
      navigateTo(redirectPath || ROUTES.HOME, { replace: true });
    } catch (err: unknown) {
      const challengeRequired = hasErrorReason(err, ERROR_REASON_CHALLENGE_REQUIRED);
      if (challengeToken || challengeRequired) {
        setChallengeToken(null);
        setChallengeResetKey((key) => key + 1);
      }
      if (hasErrorReason(err, "TOTP_REQUIRED")) {
        setNeedsCode(true);
        setCode("");
      } else if (hasErrorReason(err, "TOTP_INVALID")) {
        setNeedsCode(true);
        setCode("");
        setError({ message: t("two-factor.invalid-code"), field: "code" });
      } else if (hasConnectCode(err, Code.InvalidArgument)) {
        // The server answers an unknown user and a wrong password alike, so the message names neither.
        setNeedsCode(false);
        setCode("");
        setError({ message: t("auth.incorrect-credentials"), field: "credentials" });
      } else {
        setError({ message: getErrorMessage(err, t("error.sign-in-failed")), field: "form" });
      }
    } finally {
      actionBtnLoadingState.setFinish();
    }
  };

  const clearCredentialError = () => {
    if (error?.field === "credentials") setError(undefined);
  };

  const returnToPassword = () => {
    setNeedsCode(false);
    setCode("");
    setPassword("");
    setError(undefined);
  };

  return (
    <form className="flex w-full flex-col gap-4" onSubmit={handleFormSubmit}>
      {needsCode ? (
        <>
          <AuthNotice>{t("two-factor.sign-in-description")}</AuthNotice>
          <TwoFactorCodeField
            autoFocus
            value={code}
            onChange={(value) => {
              setCode(value);
              if (error?.field === "code") setError(undefined);
            }}
            error={error?.field === "code" ? error.message : undefined}
            readOnly={actionBtnLoadingState.isLoading}
          />
        </>
      ) : (
        <CredentialFields
          idPrefix="signin"
          username={username}
          password={password}
          passwordAutoComplete="current-password"
          readOnly={actionBtnLoadingState.isLoading}
          invalid={error?.field === "credentials"}
          onUsernameChange={(value) => {
            setUsername(value);
            clearCredentialError();
          }}
          onPasswordChange={(value) => {
            setPassword(value);
            clearCredentialError();
          }}
        />
      )}
      <ChallengeWidget onToken={setChallengeToken} resetKey={challengeResetKey} />
      {error && error.field !== "code" && <AuthNotice tone="error">{error.message}</AuthNotice>}
      <Button
        variant="primary"
        type="submit"
        disabled={needsCode && code.length !== 6}
        loading={actionBtnLoadingState.isLoading}
        loadingLabel={t("auth.signing-in")}
      >
        {t(needsCode ? "two-factor.verify" : "common.sign-in")}
      </Button>
      {needsCode && (
        <>
          <Button type="button" variant="ghost" disabled={actionBtnLoadingState.isLoading} onClick={returnToPassword}>
            {t("auth.back-to-sign-in")}
          </Button>
          <TwoFactorRecovery />
        </>
      )}
    </form>
  );
}

export default PasswordSignInForm;
