import { readFile } from "node:fs/promises";
import { expect, test } from "@playwright/test";

const admin = { username: "smoke-admin", password: "browser-smoke-password" };
const draft = "Cat_two isolated browser smoke draft";
const editedText = "Edited through the browser.";
const filename = "附件 (Cat_two).txt";
const attachment = Buffer.from("Cat_two browser smoke attachment\n");

test("sets up the instance, writes notes with attachments, and manages MCP without REST", async ({ browser, page, request, baseURL }) => {
  const retiredRequests: string[] = [];
  const internalRequests = new Set<string>();
  page.on("request", (request) => {
    const { pathname } = new URL(request.url());
    if (pathname === "/api" || pathname.startsWith("/api/")) retiredRequests.push(pathname);
    if (pathname.startsWith("/internal/")) internalRequests.add(pathname);
  });

  await test.step("Set up the first administrator and sign in from another browser session", async () => {
    const setup = await browser.newPage();
    try {
      await setup.goto(`${baseURL}/`);
      await expect(setup.getByRole("heading", { name: "初始化实例" })).toBeVisible({ timeout: 45_000 });
      await setup.getByLabel("用户名").fill(admin.username);
      await setup.getByLabel("密码", { exact: true }).fill(admin.password);
      await setup.getByRole("button", { name: "创建管理员账号", exact: true }).click();
      await expect(setup).toHaveURL(`${baseURL}/`);
    } finally {
      await setup.close();
    }

    await page.goto("/auth");
    await page.getByLabel("用户名").fill(admin.username);
    await page.getByLabel("密码", { exact: true }).fill(admin.password);
    await page.getByRole("button", { name: "登录", exact: true }).click();
    await expect(page).toHaveURL(`${baseURL}/`);
  });

  const editor = page.locator('.memo-editor-root[contenteditable="true"]');
  const card = page.getByRole("article").filter({ hasText: draft });
  await test.step("Recover browser draft text and its staged file after reloading", async () => {
    await expect(editor).toBeAttached({ timeout: 45_000 });
    await page.getByRole("button", { name: "跳过", exact: true }).click();
    await editor.click();
    await page.keyboard.insertText(draft);
    const chooser = page.waitForEvent("filechooser");
    await page.getByRole("button", { name: "添加附件", exact: true }).click();
    await (await chooser).setFiles({ name: filename, mimeType: "text/plain", buffer: attachment });
    await expect(page.getByText(filename, { exact: true })).toBeVisible();
    await expect.poll(() => page.evaluate((text) => Object.values(localStorage).some((value) => value.includes(text)), draft)).toBe(true);

    await page.reload();
    await expect(editor).toHaveText(draft);
    await expect(page.getByText(filename, { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "保存", exact: true }).click();
    await expect(card).toBeVisible();
  });

  await test.step("Edit the saved note", async () => {
    await card.getByRole("button", { name: "更多", exact: true }).click();
    await page.getByRole("menuitem", { name: "编辑", exact: true }).click();
    await card.locator('.memo-editor-root[contenteditable="true"]').press("End");
    await page.keyboard.insertText(` ${editedText}`);
    await card.getByRole("button", { name: "更新", exact: true }).click();
    await expect(card.locator('.memo-editor-root[contenteditable="true"]')).toHaveCount(0);
    await expect(card.getByText(new RegExp(editedText))).toBeVisible();
  });

  await test.step("Move the note to the recycle bin, restore it and download the saved bytes", async () => {
    await card.getByRole("button", { name: "更多", exact: true }).click();
    await page.getByRole("menuitem", { name: "删除", exact: true }).click();
    await page.getByRole("alertdialog").getByRole("button", { name: "移入回收站", exact: true }).click();
    await expect(card).toHaveCount(0);

    await page.getByRole("link", { name: "回收站", exact: true }).click();
    await expect(card.getByText(filename, { exact: true })).toBeVisible();
    await card.getByRole("button", { name: "恢复", exact: true }).click();
    await expect(card).toHaveCount(0);

    await page.goto("/");
    await expect(card).toBeVisible();
    const pendingDownload = page.waitForEvent("download");
    await card.getByRole("link").filter({ hasText: filename }).click();
    const download = await pendingDownload;
    expect(download.suggestedFilename()).toBe(filename);
    expect(await readFile(await download.path())).toEqual(attachment);
  });

  await test.step("Export the note and re-import the archive with the default skip policy", async () => {
    await page.goto("/?settings=memo-export");
    const pendingDownload = page.waitForEvent("download");
    await page.getByRole("button", { name: "导出", exact: true }).click();
    const archive = await pendingDownload;
    const bytes = await readFile(await archive.path());
    expect(archive.suggestedFilename()).toMatch(/\.zip$/);
    expect(bytes.subarray(0, 4)).toEqual(Buffer.from([0x50, 0x4b, 0x03, 0x04]));

    const chooser = page.waitForEvent("filechooser");
    await page.getByRole("button", { name: "选择文件", exact: true }).click();
    await (await chooser).setFiles({ name: archive.suggestedFilename(), mimeType: "application/zip", buffer: bytes });
    await expect(page.getByRole("radio", { name: /保留我的备忘录/ })).toBeChecked();
    await page.getByRole("button", { name: "导入 1 条备忘录", exact: true }).click();
    await expect(page.getByRole("button", { name: "导入其他文件", exact: true })).toBeVisible();
  });

  await test.step("Use the MCP service address displayed by the split Vite setup", async () => {
    await page.goto("/?settings=mcp");
    await expect(page.getByRole("heading", { name: "MCP", exact: true })).toBeVisible();
    const create = page.getByRole("button", { name: "创建 MCP 连接", exact: true });
    await create.click({ timeout: 30_000 });
    const dialog = page.getByRole("dialog", { name: "创建 MCP 连接", exact: true });
    await dialog.getByLabel("连接名称").fill("Browser smoke");
    await dialog.getByRole("radio", { name: "只读", exact: true }).click();
    await dialog.getByRole("button", { name: "创建", exact: true }).click();
    const address = dialog.getByText(`${baseURL}/mcp`, { exact: true });
    await expect(address).toBeVisible();
    const authorization = await dialog.getByText(/^Bearer cat_two_mcp_/).innerText();
    const url = await address.innerText();
    const call = {
      jsonrpc: "2.0",
      id: 3,
      method: "tools/call",
      params: { name: "memo_search", arguments: { query: draft } },
    };
    const headers = { Accept: "application/json, text/event-stream", Authorization: authorization };
    const initialized = await request.post(url, {
      headers,
      data: {
        jsonrpc: "2.0",
        id: 1,
        method: "initialize",
        params: { protocolVersion: "2025-11-25", capabilities: {}, clientInfo: { name: "cat-two-app-smoke", version: "1" } },
      },
    });
    expect(initialized.ok()).toBe(true);
    expect(await initialized.json()).toMatchObject({ result: { serverInfo: { name: expect.any(String) } } });
    const discovery = await request.post(url, { headers, data: { jsonrpc: "2.0", id: 2, method: "tools/list", params: {} } });
    expect(discovery.ok()).toBe(true);
    expect(await discovery.json()).toMatchObject({
      result: { tools: expect.arrayContaining([expect.objectContaining({ name: "memo_search" })]) },
    });
    const result = await request.post(url, { headers, data: call });
    expect(result.ok()).toBe(true);
    expect(await result.json()).toMatchObject({
      result: { structuredContent: { memos: [{ content: expect.stringContaining(`${draft} ${editedText}`) }] } },
    });
    expect((await request.post(url, { headers: { Accept: headers.Accept }, data: call })).status()).toBe(401);

    const webSettings = await request.get(`${baseURL}/internal/mcpConnections`, { headers: { Authorization: authorization } });
    expect(webSettings.status()).toBe(401);

    await dialog.getByText("关闭", { exact: true }).click();
    const connection = page.getByRole("listitem").filter({ hasText: "Browser smoke" });
    await connection.getByRole("button", { name: "删除令牌", exact: true }).click();
    const confirmation = page.getByRole("alertdialog", { name: /Browser smoke/ });
    await confirmation.getByRole("button", { name: "删除令牌", exact: true }).click();
    await expect(connection).toHaveCount(0);
    expect((await request.post(url, { headers, data: call })).status()).toBe(401);
  });

  expect(internalRequests.has("/internal/session/activity")).toBe(true);
  expect(internalRequests.has("/internal/sse")).toBe(true);
  expect(retiredRequests).toEqual([]);
});
