---
name: cat-two-ui-development
description: 设计、实现和验收 Cat_two 桌面网页界面。用于桌面布局、主题、侧栏和标签、笔记编辑和列表、设置弹窗、可访问性及浏览器回归。
---

# Cat_two 界面开发

Cat_two 是单账户自托管笔记。以桌面浏览器里的记录、检索和整理流程作为界面设计与验收范围。参考其他应用时，提取交互需求和布局问题，沿用 Cat_two 的组件、主题和状态管理，保持产品自己的界面。用户要求先确认预览时，以当前 Cat_two 页面制作预览，确认后再修改源码。 [桌面设计与配色](references/desktop-visual-design.md) · [项目开发](../cat-two-development/SKILL.md) · [产品开发](../cat-two-product-development/SKILL.md)

## 设计系统与实现约定

- 开始之前阅读安装依赖后可用的 `web/node_modules/@usememos/ui/llms.txt`。控件复用 `@usememos/ui` 的组件。组件缺少能力时，按其 `custom-components.md` 用 Base UI 补足。路由链接和自定义触发器等需要特殊元素时，使用 `buttonClasses()`、`menuClasses()`、`badgeClasses()` 保持样式和状态一致。 [主题说明](../../../web/src/themes/README.md)
- 使用 Tailwind CSS v4、`cn()` 与 `bg-mu-surface`、`text-mu-text-secondary`、`rounded-mu-md` 等语义类。旧配色名 `muted-foreground`、`destructive`、`popover` 用对应设计令牌替换。应用面板复用 `memos-panel`。字号使用 `mu-caption`、`mu-body`、`mu-label`、`mu-title` 等字级，避免 Tailwind 的字号工具类。品牌字标和随图标盒缩放的表情符号使用各自已有规则。字段字号由控件尺寸决定，各视口保持一致。 [全局样式](../../../web/src/index.css)
- 菜单使用固定密度。`Menu.Root` 和 `menuClasses()` 省略 `size`，菜单行避免用 `mu-compact` 调整密度，紧凑的三点触发按钮沿用 `sm`。
- 自定义控件用 `mu-focus` 并预留真实的 1px 边框，无边框视觉时用透明边框。保留该样式的焦点轮廓，避免叠加 `ring-*`、`focus-visible:ring-*` 或用 `outline-none` 覆盖。屏幕阅读器辅助文字用 `mu-visually-hidden`，仅聚焦时显现的控件用 `sr-only focus-visible:not-sr-only`。包组件外的 Lucide 小图标用 `mu-icon-xs`（12px）、`mu-icon-sm`（14px）或 `mu-icon`（16px），由类统一设定尺寸和笔画，避免单独使用 Tailwind 尺寸类或设置 `strokeWidth`。
- 使用 `@/` 绝对导入。Biome 使用 2 空格缩进、双引号、分号和 140 字符行宽。服务端数据放在 React Query 并复用查询 hooks，界面状态放在组件或 Context。生成的 `web/src/types/proto/` 通过 Proto 流程更新，保持在 Biome 改写范围之外。 [查询 hooks](../../../web/src/hooks/) · [仓库规范](../../../AGENTS.md)
- 用 `useTranslate` 引用文案。新增英文键时补齐所有已配置语言的译文，当前为简体中文 `zh-Hans`。复数资源用 `key_one`、`key_other` 等形式，调用时传基础键和 `count`。有意义的状态要能被键盘和辅助技术访问。为仅图标的按钮提供可读名称，分清选中、焦点、禁用和加载状态。 [翻译工具](../../../web/src/utils/i18n.ts) · [资源测试](../../../web/tests/locale-resources.test.ts)

## 桌面骨架、主题与侧栏

- `RootLayout` 管理居中的桌面工作区、侧栏、路由内容及全局弹窗。浅色和深色桌面首页的侧栏默认 360px，其他页面和主题默认 320px。拖动分隔手柄调整，双击复位。键盘聚焦手柄后可用方向键调整，Home 和 End 可到达宽度边界。宽度偏好保存在当前浏览器，渲染会按窗口限宽。浅色和深色桌面首页的单列工作区上限为 1104px，其他常规单列为 1064px，多列为 1280px，自动列或全宽由显示设置决定。 [页面骨架](../../../web/src/layouts/RootLayout.tsx) · [侧栏宽度](../../../web/src/components/AppSidebar/useSidebarWidth.ts) · [调整手柄](../../../web/src/components/AppSidebar/SidebarResizeHandle.tsx)
- 主题偏好支持浅色、深色、跟随系统和纸张。跟随系统时先解析实际配色，再设置 `<html data-theme>`。浅色主题的页面画布统一使用 `#FFFFFF`，包括首页和回收站。正文使用纯黑字，桌面首页的侧栏、账号面板、日历、输入框和笔记卡片使用纯白底色。浅色桌面首页的日历用 `#C2E8D1` 和 `#EFEFEF` 区分有笔记和无笔记日期。深色桌面首页的空日期用 `#3D3D3D`，有笔记的日期按数量显示绿色。两种模式均隐藏星期行和日期数字，今天用细边框标记。日期色块支持按日筛选，并向辅助技术提供完整日期。深色用纯白字，画布、侧栏、搜索按钮、编辑器、笔记卡片和设置面板共用 `#121212` 底色。笔记正文统一 15px 字号和 26px 行高，拉丁字符与数字优先使用内置 Barlow，中文使用平台字体回退。纸张主题沿用暖棕色配对。 [配色表](references/desktop-visual-design.md#cat_two-主题) · [日历](../../../web/src/components/ActivityCalendar/MonthCalendar.tsx) · [正文排版](../../../web/src/components/MemoEditor/README.md)
- 左侧看板使用内收的独立滚动面板，与右侧工作区留出 24px 至 32px 间隔。账户、统计、日历、标签与回收站处于同一滚动区域。浅色和深色桌面首页用 36px 区块间距和 8px 标签行间距替代面板外框及区块分隔线，其余桌面样式使用 28px 和 6px。桌面标签行高 38px，展开的标签树用引导线标出层级。默认头像使用 Cat_two 小猫图标，自定义头像优先显示。 [侧栏](../../../web/src/components/AppSidebar/AppSidebar.tsx) · [侧栏间距](../../../web/src/components/AppSidebar/sidebar-layout.ts) · [头像](../../../web/src/components/UserAvatar.tsx)
- 侧栏统计显示当前账户的回收站以外的主笔记数、不同标签路径数和记录天数。复用侧栏的统计查询，随笔记变更更新，并独立于列表筛选和排序。日期展示、日历方格、按日筛选和记录天数都以 `Asia/Shanghai` 为准。 [统计组件](../../../web/src/components/AppSidebar/CollectionStats.tsx) · [统计查询](../../../web/src/hooks/useFilteredMemoStats.ts) · [时区逻辑](../../../web/src/lib/time-zone.ts)
- 浅色与深色中，标签的置顶、树形和列表选中行使用 `#397354` 背景和白字，回收站选中使用同一行样式。纸张模式使用自身的强调色。标签计数在鼠标进入标签区或键盘焦点进入区内时可见，辅助技术仍可读取。打开回收站时右侧切换为已删除的笔记，左侧保留主页日历和标签。点日期或标签返回主页筛选。 [标签](../../../web/src/components/AppSidebar/TagsSection.tsx) · [统一行状态](../../../web/src/components/AppSidebar/SidebarRow.tsx) · [回收站](../../../web/src/pages/Trash.tsx)
- 普通标签按名称排序，保留置顶顺序。标签行菜单用确认弹窗区分仅移除标签和把笔记移入回收站。搜索弹窗提供可移除的「无标签」筛选，使用它时清除具体标签选择。 [标签菜单](../../../web/src/components/AppSidebar/TagsSection.tsx) · [搜索弹窗](../../../web/src/components/AppSidebar/QuickFindDialog.tsx)

## 笔记列表与编辑器

- 首页复用 `PagedMemoList` 的 `renderStickyHeader`，桌面滚动时保留顶部列表菜单、搜索入口和新笔记输入框。「全部笔记」标题沿用 17px 和字重 700。常规单列中，编辑器、筛选和卡片之间保持 16px 的间距。调整单列、双列、三列或自动列时，复用现有可用宽度计算，保留加载、翻页、紧凑模式和列宽不足时的回退。 [首页](../../../web/src/pages/Home.tsx) · [列表菜单](../../../web/src/components/MemoFeedMenu.tsx) · [分页列表](../../../web/src/components/PagedMemoList/PagedMemoList.tsx)
- 浅色和深色的笔记阅读区默认标签用 `#5783F7` 字色与主题标签底色。编辑器使用主题的编辑色，自定义标签沿用用户配置。 [阅读区字色](../../../web/src/components/MemoEditor/view/memo-content.css) · [标签配色](../../../web/src/lib/markdownStyles.ts)
- 日历旁的显示设置弹层以侧栏触发器为锚点，使用 `positionMethod="fixed"`。在两列模式打开弹层后滚动右侧列表，验证弹层相对触发器保持稳定。滚动左侧时，右侧笔记位置也应保持稳定。浅色和深色桌面侧栏隐藏滚动条，滚轮、触控板和键盘焦点仍可滚动到末尾的回收站。 [显示设置弹层](../../../web/src/components/MemoDisplaySettingMenu.tsx) · [侧栏滚动](../../../web/src/components/AppSidebar/AppSidebar.tsx)
- 首页从右侧输入框创建。桌面默认高度紧凑，聚焦后展开，浅色与深色的聚焦边框统一用 `rgb(48, 207, 121)`。失焦后收起，长内容在书写框内部滚动。右侧仍能在读到较早笔记时直接记录。 [输入框布局](../../../web/src/components/MemoEditor/edit/editor.css) · [首页入口](../../../web/src/pages/Home.tsx)
- 通过 `MemoEditor` 看笔记、编辑和新建，已有笔记传资源名，新笔记传草稿种子。双击某张卡片的正文或其边框内的留白，会在这张卡片内打开与菜单「编辑」相同的编辑器。卡片之间的间隙和页面空白属于列表背景。卡片内的标签、链接和待办框保留各自的点击操作。富文本与 Markdown 源码共用正文、格式操作、链接校验和渲染绑定。从底部「＋ → Markdown 模式」切换，格式快捷按钮和「更多格式」保持在输入区域下方。样式由共享主题、扩展或外层容器提供。 [编辑器结构](../../../web/src/components/MemoEditor/README.md) · [Markdown 主题](../../../web/src/lib/memo-markdown.ts) · [支持的格式](../cat-two-product-development/SKILL.md#支持的内容格式)
- 从侧栏选择日期后，新建时带入该日。筛选变更时沿用现有的时间与草稿身份规则。保存失败后保留草稿和已经上传的附件，重试复用该次请求所用的创建 ID。编辑冲突时提示复核，并保留原草稿和待传文件。成功后复用现有缓存更新与恢复路径。 [保存逻辑](../../../web/src/components/MemoEditor/edit/hooks/useMemoSave.ts) · [已有笔记查询](../../../web/src/hooks/useMemoQueries.ts)
- 从登录或最后一次笔记编辑开始计时，45 分钟无编辑时转到登录页。正文输入、格式、附件与待办更改属于编辑，同会话的多标签页共享期限。渲染、自动保存本地草稿、打开编辑器、浏览和后台同步均不计为编辑。重新登录后沿用已有草稿恢复流程。 [网页登录会话](../../../web/src/auth-state.ts)

## 设置和反馈

- 账户菜单从全局页面骨架打开设置弹窗，使用设置自身的左侧分区、右侧内容和滚动。通过 `settingSections.ts` 注册栏目和权限范围，管理员配置归入管理员分区。点关闭按钮或按 Esc 时保留笔记区域的浏览位置、筛选与草稿。从菜单打开时，关闭后返还焦点。旧版 `/setting#分区` 链接沿用相应兼容入口。 [设置弹窗](../../../web/src/components/Settings/SettingsDialog.tsx) · [栏目注册](../../../web/src/components/Settings/settingSections.ts)
- 「MCP」分区只管理 MCP 专用连接。用当前页面的 Connect 和会话请求完成设置、附件上传、回收站及 ZIP 验收。文件下载走 `/file/`。 [网页接口](../cat-two-product-development/references/external-api.md#网页接口)
- 设置左侧当前项与标签共用 `SidebarRow` 的选中样式。浅色和深色均使用 `#397354` 底色、白色文字和白色图标，纸张模式使用自身的选中色。 [设置导航](../../../web/src/components/Settings/SettingsContent.tsx) · [统一行状态](../../../web/src/components/AppSidebar/SidebarRow.tsx)
- 设置使用 `SettingSection`、`SettingGroup`、`SettingListItem` 组织标题、分组、字段与说明。采用现有的加载、空态和重试样式，错误反馈放在发生的位置。字段校验放在字段下方，使用 `Field.Error` 与 `useFieldErrors`。通用操作失败走 `showErrorToast`。敏感连接设置使用掩码输入，只显示已配置状态，留空按对应接口规则保留旧值。 [设置字段](../../../web/src/components/Settings/SettingList.tsx) · [通知](../../../web/src/components/Settings/NotificationSection.tsx) · [网络](../../../web/src/components/Settings/NetworkSection.tsx) · [错误反馈](../../../web/src/lib/toast.ts)
- 企业微信的复制入口优先显示服务端保存的公网回调地址，其次显示实例级默认值。配置来自局域网且未提供公网地址时，显示输入与保存入口。验收覆盖带端口的保存与复制、清空后恢复默认值和错误提示。 [通知设置](../../../web/src/components/Settings/NotificationSection.tsx)

## 验收

1. 从真实页面入口操作，用桌面窗口和当前改动有关的宽度验收。首先检查浅色和深色、简中和英语。修改共享主题时补验纸张主题及系统主题切换。迭代时用 `cd web && pnpm dev`，本地后端按仓库命令启动。
2. 布局改动覆盖左栏和右侧独立滚动、带大量标签时的回收站位置、隐藏计数的显现及键盘焦点。打开显示设置后滚动双列笔记，检查弹层锚点。打开、关闭设置时检查原筛选、草稿、滚动和焦点恢复。 [侧栏测试](../../../web/tests/sidebar-row-state.test.tsx) · [页面骨架测试](../../../web/tests/root-layout.test.tsx) · [分页测试](../../../web/tests/paged-memo-list.test.tsx)
3. 编辑改动覆盖输入框展开与收起、富文本和源码切换、中文输入、格式与附件。检查保存后列表和详情、重复点击或失败重试、冲突和刷新恢复。涉及日期时从侧栏选历史日期并新建，清除筛选后再回选该日期。 [编辑器测试入口](../../../web/src/components/MemoEditor/README.md#testing) · [上海日期测试](../../../web/tests/shanghai-calendar-time.test.ts)
4. 按修改范围在 `web/` 运行 `pnpm lint`、`pnpm test`（迭代时可指定相关测试文件）与 `pnpm build`。验收记录保留实际测试的页面、主题、窗口和结果，部署交付按项目 Skill 验证。 [仓库命令](../../../AGENTS.md#commands) · [项目开发](../cat-two-development/SKILL.md)
