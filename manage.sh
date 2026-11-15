#!/usr/bin/env bash
set -uo pipefail

deploy_action() (
set -Eeuo pipefail
[ "$(id -u)" -eq 0 ] || { echo '请使用 sudo 运行'; exit 1; }
. /etc/os-release
[ "$ID" = ubuntu ] || { echo '仅支持 Ubuntu；为保护当前系统，已停止。'; exit 1; }
DIR=$(cd "$(dirname "$0")" && pwd)
[ "$DIR" = /opt/pingtai ] || { echo '请先将部署包放在 /opt/pingtai'; exit 1; }
[ ! -e /etc/pingtai.env ] && [ ! -e /etc/systemd/system/pingtai.service ] && [ ! -e /etc/nginx/sites-available/pingtai ] || { echo '检测到已有平台配置，停止以防覆盖。'; exit 1; }
[ -f "$DIR/schema.sql" ] || { echo '缺少数据库结构文件'; exit 1; }
echo '将安装 Nginx、MySQL、Python 依赖，并新建数据库和服务。需要联网。'
read -r -p '确认在此 Ubuntu 测试机部署？输入 yes 继续: ' answer
[ "$answer" = yes ] || { echo '已取消'; exit 1; }
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y nginx mysql-server python3 python3-venv python3-pip
systemctl start mysql
if [ "$(mysql -Nse "SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='pingtai_test'")" != 0 ]; then echo '数据库 pingtai_test 已存在，停止以防覆盖。'; exit 1; fi
if [ "$(mysql -Nse "SELECT COUNT(*) FROM mysql.user WHERE user='pingtai_app' AND host='localhost'")" != 0 ]; then echo '数据库用户 pingtai_app 已存在，停止以防覆盖。'; exit 1; fi
if find /etc/nginx/sites-enabled -maxdepth 1 -type l ! -name default -print -quit | grep -q .; then echo '检测到其他启用的 Nginx 站点，请先检查 80 端口冲突；停止部署。'; exit 1; fi
python3 -m venv "$DIR/venv"
"$DIR/venv/bin/pip" install -r "$DIR/backend/requirements.txt" gunicorn
DBPASS=$(python3 -c 'import secrets; print(secrets.token_hex(24))')
JWT=$(python3 -c 'import secrets; print(secrets.token_hex(32))')
mysql -e "CREATE DATABASE pingtai_test CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci; CREATE USER 'pingtai_app'@'localhost' IDENTIFIED BY '$DBPASS'; GRANT ALL PRIVILEGES ON pingtai_test.* TO 'pingtai_app'@'localhost';"
mysql pingtai_test < "$DIR/schema.sql"
umask 077
cat > /etc/pingtai.env <<ENV
DATABASE_URL=mysql+pymysql://pingtai_app:${DBPASS}@localhost/pingtai_test?charset=utf8mb4
JWT_SECRET_KEY=${JWT}
ENV
chown -R root:root "$DIR"
chmod 755 /opt "$DIR" "$DIR/backend" "$DIR/frontend"
chmod -R a+rX "$DIR/backend" "$DIR/frontend" "$DIR/venv"
cat > /etc/systemd/system/pingtai.service <<SERVICE
[Unit]
Description=Pingtai Flask backend
After=network.target mysql.service
Requires=mysql.service
[Service]
User=www-data
Group=www-data
WorkingDirectory=$DIR/backend
EnvironmentFile=/etc/pingtai.env
ExecStart=$DIR/venv/bin/gunicorn --workers 2 --bind 127.0.0.1:5050 run:app
Restart=on-failure
[Install]
WantedBy=multi-user.target
SERVICE
cat > /etc/nginx/sites-available/pingtai <<'NGINX'
server {
 listen 80 default_server;
 server_name _;
 client_max_body_size 2048M;
 root /opt/pingtai/frontend;
 index index.html;
 location / { try_files $uri $uri/ /index.html; }
 location /api/ { proxy_pass http://127.0.0.1:5050/; proxy_set_header Host $host; proxy_set_header X-Real-IP $remote_addr; }
}
NGINX
[ ! -L /etc/nginx/sites-enabled/default ] || rm /etc/nginx/sites-enabled/default
ln -s /etc/nginx/sites-available/pingtai /etc/nginx/sites-enabled/pingtai
nginx -t
systemctl daemon-reload
systemctl enable --now pingtai
systemctl enable nginx
systemctl reload nginx || systemctl start nginx
echo '部署完成。空数据库尚无用户，请运行 sudo /opt/pingtai/manage.sh，选择“设置管理员密码” 创建 admin 并设置密码。'

)

start_action() (
set -euo pipefail
[ "$(id -u)" -eq 0 ] || { echo '请使用 sudo 运行'; exit 1; }
systemctl start mysql
systemctl start pingtai
nginx -t
systemctl start nginx
systemctl is-active pingtai nginx

)

stop_action() (
set -euo pipefail
[ "$(id -u)" -eq 0 ] || { echo '请使用 sudo 运行'; exit 1; }
systemctl stop pingtai
# 不停止共享的 Nginx 或 MySQL，以免影响其他服务；代理请求将不可用。
echo '平台后端已停止；Nginx 和 MySQL 保持运行。'

)

password_action() (
set -euo pipefail
[ "$(id -u)" -eq 0 ] || { echo '请使用 sudo 运行'; exit 1; }
[ -f /etc/pingtai.env ] || { echo '未找到部署配置，请先通过管理菜单部署'; exit 1; }
DIR=$(cd "$(dirname "$0")" && pwd)
cd "$DIR/backend"
set -a; . /etc/pingtai.env; set +a
exec "$DIR/venv/bin/python" - <<'PY'
import datetime, getpass, hashlib, sys
from app import create_app, db
from app.models.user import User
p=getpass.getpass('设置 admin 新密码: ')
q=getpass.getpass('再次输入新密码: ')
if not p or p != q: sys.exit('密码为空或两次不一致，未修改。')
app=create_app()
with app.app_context():
    user=User.query.filter_by(username='admin').one_or_none()
    if user is None:
        if User.query.filter_by(usertype=1).first(): sys.exit('已存在其他管理员，未创建 admin。')
        now=datetime.datetime.now()
        user=User(username='admin',usertype=1,status=1,deltag=0,created_at=now,updated_at=now)
        db.session.add(user)
    user.password=hashlib.md5(p.encode()).hexdigest()
    user.updated_at=datetime.datetime.now()
    try: db.session.commit()
    except Exception:
        db.session.rollback(); sys.exit('保存失败；可能与已有用户的密码摘要或权限冲突，未完成修改。')
    print('admin 密码设置成功。')
PY

)

export -f deploy_action start_action stop_action password_action
[ "$(id -u)" -eq 0 ] || { echo '请使用 sudo 运行此脚本'; exit 1; }
while true; do
    printf '\n========== 平台管理 ==========\n1. 部署（Ubuntu 测试机）\n2. 启动服务\n3. 停止平台后端\n4. 设置管理员密码\n0. 退出\n'
    read -r -p '请选择: ' choice || { echo; exit 0; }
    case "$choice" in
        1) bash -e -c deploy_action || echo '部署未完成，请检查上述错误；可能存在已创建的资源。' ;;
        2) bash -e -c start_action || echo '启动失败，请检查上述错误。' ;;
        3) bash -e -c stop_action || echo '停止失败，请检查上述错误。' ;;
        4) bash -e -c password_action || echo '密码设置未完成，请检查上述错误。' ;;
        0) echo '已退出'; exit 0 ;;
        *) echo '无效选项，请重新输入。' ;;
    esac
done
