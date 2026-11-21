import { Link, useSearchParams } from "react-router-dom";
import AuthNotice from "@/components/AuthNotice";
import AuthPageLayout, { AuthLinkPrompt } from "@/components/AuthPageLayout";
import PasswordSignInForm from "@/components/PasswordSignInForm";
import { useInstance } from "@/contexts/InstanceContext";
import { ROUTES } from "@/router/routes";
import {
  AUTH_REASON_PARAM,
  AUTH_REASON_SIGNUPS_CLOSED,
  AUTH_REDIRECT_PARAM,
  appendSearchParams,
  getSafeRedirectPath,
} from "@/utils/auth-redirect";
import { useTranslate } from "@/utils/i18n";

const SignIn = () => {
  const t = useTranslate();
  const { generalSetting: instanceGeneralSetting } = useInstance();
  const [searchParams] = useSearchParams();
  const redirectTarget = getSafeRedirectPath(searchParams.get(AUTH_REDIRECT_PARAM));
  const signUpPath = appendSearchParams(ROUTES.AUTH_SIGNUP, searchParams);

  const registrationOpen = !instanceGeneralSetting.disallowUserRegistration;
  const cameFromClosedSignUp = searchParams.get(AUTH_REASON_PARAM) === AUTH_REASON_SIGNUPS_CLOSED;

  if (instanceGeneralSetting.disallowPasswordAuth) {
    return (
      <AuthPageLayout title={t("auth.signin-unavailable-title")} description={t("auth.signin-unavailable-description")}>
        <Link to={ROUTES.AUTH_ADMIN} className="mu-body text-mu-accent hover:underline" viewTransition>
          {t("auth.admin-sign-in")}
        </Link>
      </AuthPageLayout>
    );
  }

  return (
    <AuthPageLayout title={t("common.sign-in")}>
      {cameFromClosedSignUp && <AuthNotice className="-mt-2 mb-6">{t("auth.signups-closed-description")}</AuthNotice>}
      <PasswordSignInForm redirectPath={redirectTarget} />
      {registrationOpen && !cameFromClosedSignUp && (
        <AuthLinkPrompt prompt={t("auth.sign-up-tip")} to={signUpPath} label={t("common.sign-up")} />
      )}
    </AuthPageLayout>
  );
};

export default SignIn;
