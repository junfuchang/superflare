# 配置说明

`config.yml` 对应当前 `config/model/application.go`。

## 基础信息

- `Title`
- `Footer`
- `SiteIcon`
- `SiteIconMode`
- `Locale`

## 首页模块

- `ShowTitle`
- `Greetings`
- `ShowSearchComponent`
- `DisabledSearchAutoFocus`
- `ShowDateTime`
- `ShowApps`
- `ShowBookmarks`
- `AppsTitle`
- `BookmarksTitle`

## 行为设置

- `OpenAppNewTab`
- `OpenBookmarkNewTab`
- `HideSettingButton`
- `HideHelpButton`
- `EnableEncryptedLink`
- `IconMode`
- `KeepLetterCase`

## 书签显示

- `BookmarkCategoryColor`
- `BookmarkItemColor`

## 主题与背景

- `Theme`
- `CustomThemeBackground`
- `CustomThemePrimary`
- `CustomThemeAccent`
- `BackgroundImage`
- `BackgroundImageMode`
- `BackgroundBlur`
- `BackgroundOpacity`
- `GlassEffect`
- `GlassIntensity`

## 布局

- `HomeMaxColumns`
- `HomeMaxWidth`

## 登录

- `LoginUser`
- `LoginPass`

## 说明

- 留空值会回退到默认配置或运行参数
- `LoginUser` / `LoginPass` 留空时，仍可由环境变量或命令行覆盖
- 运行时数据文件默认位于仓库根目录，不建议另建旧式 `app/` 子目录结构
