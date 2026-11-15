import datetime
from flask import Blueprint, request, jsonify, current_app
from flask_jwt_extended import create_access_token, jwt_required, get_jwt_identity, create_refresh_token, get_jwt
from functools import wraps
from ..models.user import User, db
import hashlib
from .. import jwt_blocklist  # 导入黑名单集合

auth = Blueprint('auth', __name__)

def admin_required(fn):
    @wraps(fn)
    def wrapper(*args, **kwargs):
        current_user = get_jwt_identity()
        user = User.query.filter_by(username=current_user).first()
        if user is None or user.usertype != 1:
            return jsonify({"errcode":401, "errmsg": "权限受限，非管理员用户"})
        else:                  
            return fn(*args, **kwargs)
    return wrapper

@auth.route('/addUser', methods=['POST'])
@jwt_required()
@admin_required
def add_user():
    req_dict = request.get_json()
    username = req_dict.get('username')
    password = req_dict.get('password')
    usertype = req_dict.get('usertype')
    if not all([username, password, usertype]):
        return jsonify({"errcode": "501", "errmsg": "缺少参数"})
    else:
        if User.query.filter_by(username=username).first():
            return jsonify({"errcode": "501", "errmsg": "用户已存在"})
        hash_password = hashlib.md5(password.encode()).hexdigest()
        data = User(username=username, usertype=int(usertype), password=hash_password, status=1,deltag=0,
                created_at=datetime.datetime.now(),updated_at=datetime.datetime.now())
        db.session.add(data)
        db.session.commit()
        return jsonify(errcode=200,errmsg='添加成功')
    
@auth.route('/login', methods=['POST'])
def login():
    username = request.form.get('username')
    password = request.form.get('password')
    if not all([username, password]):
        return jsonify({"errcode":401,"errmsg":"用户名或密码为空"})
    users = User.query.filter_by(username = username).first()
    if not users:
        return jsonify({"errcode":401,"errmsg":"用户不存在"})
    if users is None or users.password != hashlib.md5(password.encode()).hexdigest():
        return jsonify({"errcode":401,"errmsg":"密码错误"})
    else:
        userType = users.usertype
        access_token = create_access_token(identity=username)
        refresh_token = create_refresh_token(identity=username)
        current_time = datetime.datetime.now()
        future_time = current_time + datetime.timedelta(minutes=30)
        return jsonify(
                        access_token=access_token,
                        refresh_token=refresh_token,
                        user_type=userType,
                        token_expiration = int(future_time.timestamp()),
                        created_at= users.created_at,
                        bind_wx = 1 if users.openid else 0,
                        errcode=200,errmsg='登录成功')
    
@auth.route('/refresh', methods=['POST'])
@jwt_required(refresh=True)
def refresh():
    identity = get_jwt_identity()
    access_token = create_access_token(identity=identity)
    return jsonify(access_token=access_token,errcode=200,errmsg='success')

@auth.route('/logout', methods=['GET'])
@jwt_required()
def logout():
    jti = get_jwt()["jti"]
    jwt_blocklist.add(jti)
    return jsonify({
        "code": 200,
        "msg": "退出登录成功"
    })

@auth.route('/delUser', methods=['POST'])
@jwt_required()
@admin_required
def del_user():
    user = get_jwt_identity()
    id=request.json.get("id")
    data = User.query.filter_by(id=id).first()
    if not data:
        return jsonify({"errcode": "501", "errmsg": "用户不存在"})
    if data.username == user:
        return jsonify({"errcode": "501", "errmsg": "不能删除自己"})
    else:
        data.deltag = 1
        db.session.commit()
        return jsonify(errcode=200,errmsg='删除成功')
    
@auth.route('/get_user_list', methods=['GET'])
@jwt_required()
@admin_required
def get_user_list():
    pagesize = request.args.get("pagesize", 10)
    pagenum = request.args.get("pagenum", 1)
    total = User.query.filter_by(deltag=0).count()
    page_objs = User.query.filter_by(deltag=0).paginate(
        page = int(pagenum),
        per_page = int(pagesize),
        error_out=False,
        max_per_page=50
    ).items
    data = []
    for obj in page_objs:
        data.append({
            "id": obj.id,
            "username": obj.username,
            "usertype": obj.usertype,
            "status": obj.status,
        })
    return jsonify({"data":data, "total":total, "errcode": "200", "errmsg": "success", "pagenum":pagenum, "pagesize":pagesize})

@auth.route('/editUser', methods=['POST'])
@jwt_required()
def edit_user():
    req_dict = request.get_json()
    id = req_dict.get('id')
    password = req_dict.get('password')
    if not all([id, password]):
        return jsonify({"errcode": "501", "errmsg": "缺少参数"})
    try:
        hash_password = hashlib.md5(password.encode()).hexdigest()
        data = User.query.filter_by(id=id).first()
        data.password = hash_password
        db.session.commit()
        return jsonify(errcode=200,errmsg='修改成功')
    except Exception as e:
        return jsonify({"errcode": "501", "errmsg": "修改失败"})