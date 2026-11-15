# 超凡AI问答平台 · Ubuntu 部署说明

## 新版 Go 服务：网页配置

新版发布包为 `dist/pingtai-release.zip`。Windows 解压后双击 `start.bat`，浏览器会自动打开配置页面；Linux 启动 `./manage-go.sh start` 后，使用启动日志中的一次性配置链接进入网页。数据库、初始管理员和模型接口均在网页填写，无需终端交互配置。

保存后重启服务即可加载配置，后续通过 `/setup` 使用管理员账号修改。详细说明见 [Go 版本部署说明](README_GO.md)。以下内容仅适用于旧版 Python 部署脚本。

## 旧版 Python 部署

部署包包含前后端文件及 MySQL 表结构，不包含原数据库业务数据、原环境配置或凭据。前端的 ai.zip 是原前端构建包，不是后端源码包。

在隔离的 Ubuntu 测试机上，将压缩包复制至 /opt 并执行：

    sudo unzip /opt/pingtai.zip -d /opt
    sudo /opt/pingtai/manage.sh

在交互菜单中选择 1 部署，再选择 4 创建 admin 并设置密码；密码在终端隐藏输入。菜单也提供启动和停止功能。停止操作只停止平台后端，不停止可能被其他服务共享的 Nginx 和 MySQL。

部署需要联网、apt 和 systemd，会安装 Nginx、MySQL、Python 及依赖，新建 pingtai_test 数据库和 pingtai_app 数据库账号，并更改默认 Nginx 站点。遇到同名数据库、服务配置或其他启用的 Nginx 站点时脚本会停止，不覆盖。部署失败可能留下已安装软件、数据库或配置，不会自动回滚，请人工检查。

外部 Coze 等服务的私钥及配置未包含，相关功能需另行配置。原应用使用 MD5 保存密码，安全性较弱，不建议直接用于生产。脚本尚未在 Ubuntu 上完成端到端验证。
