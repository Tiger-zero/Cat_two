import { useSearchParams } from "react-router-dom";
import AuthNotice from "@/components/AuthNotice";
import AuthPageLayout from "@/components/AuthPageLayout";
import PasswordSignInForm from "@/components/PasswordSignInForm";
import {
  AUTH_REASON_IDLE_SESSION,
  AUTH_REASON_PARAM,
  AUTH_REASON_SIGNUPS_CLOSED,
  AUTH_REDIRECT_PARAM,
  getSafeRedirectPath,
} from "@/utils/auth-redirect";
import { useTranslate } from "@/utils/i18n";

const SignIn = () => {
  const t = useTranslate();
  const [searchParams] = useSearchParams();
  const redirectTarget = getSafeRedirectPath(searchParams.get(AUTH_REDIRECT_PARAM));
  const cameFromClosedSignUp = searchParams.get(AUTH_REASON_PARAM) === AUTH_REASON_SIGNUPS_CLOSED;
  const expiredFromInactivity = searchParams.get(AUTH_REASON_PARAM) === AUTH_REASON_IDLE_SESSION;

  return (
    <AuthPageLayout title={t("common.sign-in")}>
      {cameFromClosedSignUp && <AuthNotice className="-mt-2 mb-6">{t("auth.signups-closed-description")}</AuthNotice>}
      {expiredFromInactivity && <AuthNotice className="-mt-2 mb-6">{t("auth.session-idle-expired")}</AuthNotice>}
      <PasswordSignInForm redirectPath={redirectTarget} />
    </AuthPageLayout>
  );
};

export default SignIn;
