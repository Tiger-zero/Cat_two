import { create } from "@bufbuild/protobuf";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import InstanceSection from "@/components/Settings/InstanceSection";
import { InstanceSetting_Key } from "@/types/proto/api/instance_service_pb";
import { type User, User_Role, UserSchema } from "@/types/proto/api/user_service_pb";

const instance = vi.hoisted(() => ({
  generalSetting: {
    additionalScript: "legacy script source",
    customProfile: undefined,
    disallowChangeNickname: true,
    disallowChangeUsername: false,
    disallowPasswordAuth: false,
    disallowUserRegistration: true,
    weekStartDayOffset: 0,
  },
  fetchSetting: vi.fn(),
  updateSetting: vi.fn(),
}));
const account = vi.hoisted(() => ({ user: undefined as User | undefined, update: vi.fn() }));

vi.mock("@/contexts/InstanceContext", () => ({
  useInstance: () => instance,
}));

vi.mock("@/hooks/useCurrentUser", () => ({ default: () => account.user }));
vi.mock("@/hooks/useUserQueries", () => ({ useUpdateUser: () => ({ mutateAsync: account.update }) }));

vi.mock("@/components/UpdateCustomizedProfileDialog", () => ({ default: () => null }));

vi.mock("@/utils/i18n", () => ({
  useTranslate: () => (key: string) => key,
}));

describe("<InstanceSection> access setting", () => {
  beforeEach(() => {
    account.user = create(UserSchema, { name: "users/admin", displayName: "Administrator", role: User_Role.ADMIN });
    account.update.mockReset().mockResolvedValue(undefined);
    instance.fetchSetting.mockReset().mockResolvedValue(undefined);
    instance.updateSetting.mockReset().mockResolvedValue(undefined);
  });

  it("opens the existing password dialog for the signed-in administrator", async () => {
    render(<InstanceSection />);

    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "setting.account.change-password" }));

    const dialog = within(await screen.findByRole("dialog", { name: "setting.account.change-password (Administrator)" }));
    fireEvent.change(dialog.getByPlaceholderText("auth.new-password"), { target: { value: "replacement-password" } });
    fireEvent.change(dialog.getByPlaceholderText("auth.repeat-new-password"), { target: { value: "replacement-password" } });
    fireEvent.click(dialog.getByRole("button", { name: "common.save" }));

    await waitFor(() =>
      expect(account.update).toHaveBeenCalledWith({
        user: { name: "users/admin", password: "replacement-password" },
        updateMask: ["password"],
      }),
    );
    expect(instance.updateSetting).not.toHaveBeenCalled();
  });

  it("omits the administrator password entry for a regular account", () => {
    account.user = create(UserSchema, { name: "users/member", role: User_Role.USER });
    render(<InstanceSection />);

    expect(screen.queryByRole("button", { name: "setting.account.change-password" })).not.toBeInTheDocument();
  });

  it("leaves general settings editable without altering the retired access fields", async () => {
    render(<InstanceSection />);

    const save = screen.getByRole("button", { name: "common.save" });
    expect(save).toBeDisabled();
    fireEvent.click(screen.getByRole("combobox"));
    const monday = await screen.findByRole("option", { name: "setting.instance.monday" });
    fireEvent.pointerDown(monday, { pointerType: "mouse" });
    fireEvent.click(monday);
    expect(save).toBeEnabled();
    fireEvent.click(save);

    await waitFor(() => expect(instance.updateSetting).toHaveBeenCalledTimes(1));
    const setting = instance.updateSetting.mock.calls[0][0];
    expect(setting.name).toBe("instance/settings/GENERAL");
    expect(setting.value.case).toBe("generalSetting");
    expect(setting.value.value).toMatchObject({
      additionalScript: "",
      weekStartDayOffset: 1,
      disallowUserRegistration: true,
      disallowPasswordAuth: false,
      disallowChangeUsername: false,
      disallowChangeNickname: true,
    });
    expect(instance.fetchSetting).toHaveBeenCalledWith(InstanceSetting_Key.GENERAL);
  });

  it("groups calendar settings and their save button before administrator access", () => {
    render(<InstanceSection />);

    const groupHeadings = screen.getAllByRole("heading", { level: 2 });
    expect(groupHeadings.map((heading) => heading.textContent)).toEqual(["common.basic", "setting.instance.access-title"]);

    const weekStartDay = screen.getByText("setting.instance.week-start-day");
    const accessHeading = screen.getByRole("heading", { level: 2, name: "setting.instance.access-title" });
    expect(weekStartDay.compareDocumentPosition(accessHeading) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    const save = screen.getByRole("button", { name: "common.save" });
    expect(save.compareDocumentPosition(accessHeading) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
});
