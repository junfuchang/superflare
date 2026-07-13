(function () {
  if (typeof introJs !== "function") {
    return;
  }

  function step(selector, title, intro) {
    var item = { title: title, intro: intro };
    if (selector) {
      var element = document.querySelector(selector);
      if (element && document.body.contains(element)) {
        item.element = element;
      }
    }
    return item;
  }

  var steps = [
    step(
      "",
      "SuperFlare 使用向导",
      "这里会快速介绍首页、搜索、在线编辑、外观设置和部署相关入口。你可以随时跳过，后续也能从帮助页重新打开。"
    ),
    step(
      "#search-container",
      "搜索模式",
      "搜索框既可以用于页内检索应用与书签，也可以切换为搜索引擎模式。搜索引擎支持预设和自定义模板，默认使用 Bing，并可选择当前页打开或新标签页打开。"
    ),
    step(
      "#hero-container",
      "首页信息",
      "首页顶部可展示问候语、时间和自定义标题。问候语支持固定内容、按时段展示，也支持多条内容在刷新时随机展示。"
    ),
    step(
      "#container-apps",
      "应用与书签",
      "应用适合放常用入口，书签适合按分类沉淀更多链接。标题文本、最大列数、展示宽度、图标显示规则和字体颜色都可以在设置中调整。"
    ),
    step(
      "#container-bookmakrs",
      "书签分类与子目录",
      "书签支持分类和子目录。子目录会显示在分类顶部，点击后可以展开或折叠其中的书签，适合整理 NAS、开发、影音等多层入口。"
    ),
    step(
      ".toolbar-container",
      "设置、帮助和提示入口",
      "页面角落提供设置、帮助、提示信息等入口。部分入口可以在设置页中开启或关闭，避免首页展示过多不常用按钮。"
    ),
    step(
      "",
      "图标显示规则",
      "图标可手动填写，也可以在未配置时自动尝试读取站点 favicon。获取失败时会回退到内置 bookmark 图标；也可以设置为空图标不展示或全部不展示图标。"
    ),
    step(
      "",
      "在线编辑",
      "进入 /editor 可以编辑分类、应用、书签和子目录，支持链接有效性检查、打开图标页、保存提示、数据备份与恢复导入。"
    ),
    step(
      "",
      "备份与恢复",
      "编辑页提供数据导出和恢复导入。建议在大批量调整书签前先导出备份，恢复失败时会给出操作提示，便于定位格式问题。"
    ),
    step(
      "",
      "主题与背景",
      "设置页支持预设主题、自定义 RGBA 颜色、远程或本地背景图、背景毛玻璃和液态玻璃效果，也可以定制网站标题、网站图标、页脚和首页宽度。"
    ),
    step(
      "",
      "端口与安全",
      "端口页用于查看本机端口信息；未开启登录功能时会隐藏该页面以减少信息外泄。建议上线后修改默认登录凭据，并保持 Cookie Secret 固定且足够随机。"
    ),
    step(
      "",
      "部署方式",
      "SuperFlare 支持 Windows/Linux 原生运行、Docker 镜像和 fnOS 应用。Docker 如需读取宿主机端口信息，需要挂载 /proc 并设置 FLARE_PORT_PROC_ROOT；fnOS 应用会优先保留已有配置。"
    ),
  ];

  introJs()
    .setOptions({
      doneLabel: "完成",
      prevLabel: "上一步",
      nextLabel: "下一步",
      skipLabel: "跳过",
      showProgress: true,
      scrollToElement: true,
      steps: steps,
    })
    .start();
})();
