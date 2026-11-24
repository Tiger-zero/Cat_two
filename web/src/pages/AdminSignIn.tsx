import { useLocation } from "react-router-dom";
import AuthPageLayout, { AuthBackLink } from "@/components/AuthPageLayout";
import PasswordSignInForm from "@/components/PasswordSignInForm";
import { ROUTES } from "@/router/routes";
import { AUTH_REDIRECT_PARAM, getSafeRedirectPath } from "@/utils/auth-redirect";
import { useTranslate } from "@/utils/i18n";

const AdminSignIn = () => {
  const t = useTranslate();
  const { search, hash } = useLocation();
  const redirectPath = getSafeRedirectPath(new URLSearchParams(search).get(AUTH_REDIRECT_PARAM));

  return (
    <AuthPageLayout title={t("auth.admin-sign-in")}>
      <PasswordSignInForm redirectPath={redirectPath} />
      <AuthBackLink to={`${ROUTES.AUTH}${search}${hash}`} />
    </AuthPageLayout>
  );
};

export default AdminSignIn;
