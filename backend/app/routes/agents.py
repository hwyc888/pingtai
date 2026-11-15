from flask import Blueprint, request, jsonify, current_app, Response
from functools import wraps
from cozepy import COZE_COM_BASE_URL
from flask_jwt_extended import create_access_token, jwt_required, get_jwt_identity, create_refresh_token, get_jwt
from .. import pub
from ..models.user import User, db
import base64
import json
import requests

agents = Blueprint('agents', __name__)

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

# 获取智能体配置
@agents.route('/getAgentConfig', methods=['GET'])
@jwt_required()
@admin_required
def get_agent_config():
    bot_id = request.args.get('bot_id')
    if bot_id is None:
        return jsonify({'code':500, 'msg': '缺少智能体ID参数'})
    url = 'https://api.coze.cn/v1/bot/get_online_info?bot_id='+bot_id
    response = pub.request(
        method='GET',
        url=url
    )
    return jsonify(response)

# 更新智能体
@agents.route('/updateAgent', methods=['POST'])
@jwt_required()
@admin_required
def update_agent():
    bot_id = request.form.get('bot_id')
    if bot_id is None:
        return jsonify({'code':500, 'msg': '缺少智能体ID参数'})
    name = request.form.get('name')
    description = request.form.get('description')
    icon = request.files.get('icon')
    prompt = request.form.get('prompt', None)
    prologue = request.form.get('prologue', None)
    suggested_questions = request.form.getlist('suggested_questions')
    dataset_ids = request.form.getlist('dataset_ids')

    data = {
        "bot_id": bot_id,
        "name": name,
        "description": description
    }
    if prologue or (suggested_questions and len(suggested_questions) > 0 and suggested_questions[0]):
        data["onboarding_info"] = {}
        if prologue:
            data["onboarding_info"]["prologue"] = prologue
        if suggested_questions and len(suggested_questions) > 0 and suggested_questions[0]:
            suggested_questions = [str(num) for num in suggested_questions]
            data["onboarding_info"]["suggested_questions"] = suggested_questions
    if dataset_ids and len(dataset_ids) > 0 and dataset_ids[0]:
        data["knowledge"] = {}
        dataset_ids = [str(num) for num in dataset_ids]
        data["knowledge"]["dataset_ids"] = dataset_ids
    if prompt:
        data["prompt_info"] = {"prompt": prompt}
    if icon:
        icon_file_id = upload_file(icon)
        data['icon_file_id'] = icon_file_id
    url = 'https://api.coze.cn/v1/bot/update'
    response = pub.request(
        method='POST',
        url=url,
        data=data
    )
    return jsonify(response)

# 发布智能体
@agents.route('/publishAgent', methods=['POST'])
@jwt_required()
@admin_required
def publish_agent():
    bot_id = request.json.get('bot_id')
    if bot_id is None:
        return jsonify({'code':500, 'msg': '缺少智能体ID参数'})
    url = 'https://api.coze.cn/v1/bot/publish'
    data = {
        "bot_id": bot_id,
        "connector_ids": ["1024","999"]
    }
    response = pub.request(
        method='POST',
        url=url,
        data=data
    )
    return jsonify(response)

# 获取智能体列表
@agents.route('/getAgentsList', methods=['GET'])
@jwt_required()
@admin_required
def get_agents_list():
    space_id = request.args.get('space_id')
    if space_id is None:
        return jsonify({'code':500, 'msg': '缺少空间ID参数'})
    page_size = request.args.get('page_size', 10)
    page_num = request.args.get('page_num', 1)
    url = 'https://api.coze.cn/v1/space/published_bots_list?space_id='+space_id+'&page_size='+page_size+'&page_index='+page_num
    response = pub.request(
        method='GET',
        url=url
    )
    return jsonify(response)

# 创建会话
@agents.route('/createConversation', methods=['POST'])
@jwt_required()
def create_session():
    url = "https://api.coze.cn/v1/conversation/create"
    response = pub.request(
        method='POST',
        url=url
    )
    return jsonify(response)

# 获取会话
@agents.route('/getConversation', methods=['GET'])
@jwt_required()
def get_session():
    conversation_id = request.args.get('conversation_id')
    if conversation_id is None:
        return jsonify({'code':500, 'msg': '缺少会话ID参数'})
    url = "https://api.coze.cn/v1/conversation/retrieve?conversation_id="+conversation_id
    response = pub.request(
        method='GET',
        url=url
    )
    return jsonify(response)

# 发起对话
@agents.route('/chat', methods=['POST'])
@jwt_required()
def start_session():
    # 提前获取请求数据
    conversation_id = request.form.get('conversation_id')
    bot_id = request.form.get('bot_id')
    content = request.form.get('content')
    stream = request.form.get('stream')
    if conversation_id is None:
        return jsonify({'code': 500, 'msg': '缺少会话ID参数'})
    if bot_id is None:
        return jsonify({'code': 500, 'msg': '缺少智能体ID参数'})
    if content is None:
        return jsonify({'code': 500, 'msg': '缺少消息内容'})

    url = request.form.get('url')
    file = request.files.get('file')
    user_id = get_jwt_identity()

    data = {
        "bot_id": bot_id,
        "user_id": user_id,
        "stream": True if stream == '1' else False,
        "auto_save_history": True,
    }

    if not url and not file:
        data['additional_messages'] = [{
            "role": "user",
            "content_type": "text",
            "content": content
        }]
    else:
        up_type = request.form.get('up_type')
        if not up_type:
            return jsonify({'code': 500, 'msg': '缺少上传类型参数'})
        up_content = []
        if up_type == 'image':
            if url:
                up_content.append({
                    "type": "image",
                    "file_url": url
                })
            else:
                file_id = upload_file(file)
                if file_id:
                    up_content.append({
                        "type": "image",
                        "file_id": file_id
                    })
                else:
                    return jsonify({'code': 500, 'msg': '上传文件失败'})
        elif up_type == 'file':
            if url:
                up_content.append({
                    "type": "file",
                    "file_url": url
                })
            else:
                file_id = upload_file(file)
                if file_id:
                    up_content.append({
                        "type": "file",
                        "file_id": file_id
                    })
                else:
                    return jsonify({'code': 500, 'msg': '上传文件失败'})
        up_content.append({
            "role": "user",
            "content_type": "text",
            "content": content
        })
        data['additional_messages'] = [{
            "role": "user",
            "content_type": "object_string",
            "content": str(up_content).replace("'", '"')
        }]
    
    api_url = "https://api.coze.cn/v3/chat?conversation_id=" + conversation_id
    if data['stream']:
        # data["stream"] = str(data["stream"]).lower()
        # data["auto_save_history"] = str(data["auto_save_history"]).lower()  # 布尔转换为字符串
        headers = {
            'Authorization': pub.get_coze_token(),
            'Content-Type': 'application/json'
        }
        # print(data)
        
        # 使用流式请求
        response = requests.request(
            method='POST',
            url=api_url,
            data=json.dumps(data),
            headers=headers,
            stream=True,  # 开启流式请求
            timeout=30
        )
        response.raise_for_status()
        # 处理流式响应
        def generate():
            for res in response.iter_lines():
                if res:
                    yield json.dumps({
                        "code": 200,
                        "msg": "success",
                        "data": res.decode('utf-8')  # 解码为字符串
                    }) + "\n"  # 添加换行符以分隔每个 JSON 对象

        return Response(generate(), mimetype='application/json')  # 返回流式响应
    else:
        response = pub.request(
            method='POST',
            url=api_url,
            data=data
        )
        return jsonify(response)  # 确保返回的是可以序列化的对象


# 查看对话详情
@agents.route('/getChatDetail', methods=['GET'])
@jwt_required()
def get_conversation_detail():
    conversation_id = request.args.get('conversation_id')
    chat_id = request.args.get('chat_id')
    if conversation_id is None:
        return jsonify({'code':500, 'msg': '缺少会话ID参数'})
    url = "https://api.coze.cn/v3/chat/retrieve?conversation_id="+conversation_id+"&chat_id="+chat_id
    response = pub.request(
        method='GET',
        url=url
    )
    return jsonify(response)

# 查看对话消息详情
@agents.route('/getChatMessage', methods=['GET'])
@jwt_required()
def get_chat_message():
    conversation_id = request.args.get('conversation_id')
    chat_id = request.args.get('chat_id')
    if conversation_id is None:
        return jsonify({'code':500, 'msg': '缺少会话ID参数'})
    if chat_id is None:
        return jsonify({'code':500, 'msg': '缺少消息ID参数'})
    url = "https://api.coze.cn/v3/chat/message/list?conversation_id="+conversation_id+"&chat_id="+chat_id
    response = pub.request(
        method='GET',
        url=url
    )
    return jsonify(response)


# 创建消息
@agents.route('/createMessage', methods=['POST'])
@jwt_required()
def create_message():
    conversation_id = request.form.get('conversation_id')
    if conversation_id is None:
        return jsonify({'code':500, 'msg': '缺少会话ID参数'})
    content = request.form.get('content')
    if content is None:
        return jsonify({'code':500, 'msg': '缺少消息内容'})
    url = request.form.get('url')
    file = request.files.get('file')

    data = {
        "role": 'user'
    }
    if not url and not file:
        data['content_type'] = 'text'
        data['content'] = content
    else:
        up_type = request.form.get('up_type')
        if not up_type:
            return jsonify({'code':500, 'msg': '缺少上传类型参数'})
        data['content_type'] = 'object_string'
        up_content = [{
            "type": "text",
            "text": content
        }]
        if up_type == 'image':
            if url:
                up_content.append(
                    {
                        "type": "image",
                        "file_url": url
                    }
                )
            else:
                file_id = upload_file(file)
                if file_id:
                    up_content.append(
                        {
                            "type": "image",
                            # "file_id": upload_file(base64.b64encode(file.read()).decode('utf-8'))
                            "file_id": file_id
                        }
                    )
                else:
                    return jsonify({'code':500, 'msg': '上传文件失败'})
        elif up_type == 'file':
            if url:
                up_content.append(
                    {
                        "type": "file",
                        "file_url": url
                    }
                )
            else:
                file_id = upload_file(file)
                if file_id:
                    up_content.append(
                        {
                            "type": "file",
                            "file_id": file_id
                        }
                    )
                else:
                    return jsonify({'code':500, 'msg': '上传文件失败'})
        data['content'] = str(up_content).replace("'", '"')
        # data['content'] = "[{\"type\":\"text\",\"text\":\"帮我看看这个图片里有什么内容？\"},{\"type\":\"image\",\"file_id\":\"7478984714918445096\"}]"
    api_url = "https://api.coze.cn/v1/conversation/message/create?conversation_id="+conversation_id
    response = pub.request(
        method='POST',
        url=api_url,
        data=data
    )
    return jsonify(response)

# 查看消息列表
@agents.route('/getMessageList', methods=['POST'])
@jwt_required()
def get_message_list():
    conversation_id = request.json.get('conversation_id')
    if conversation_id is None:
        return jsonify({'code':500, 'msg': '缺少会话ID参数'})
    url = "https://api.coze.cn/v1/conversation/message/list?conversation_id="+conversation_id
    order = request.json.get('order','desc')
    limit = request.json.get('limit',50)
    data = {
        "order": order,
        "limit": limit
    }
    response = pub.request(
        method='POST',
        url=url,
        data=data
    )
    return jsonify(response)

# 查看消息详情
@agents.route('/getMessage', methods=['GET'])
@jwt_required()
def get_message():
    conversation_id = request.args.get('conversation_id')
    if conversation_id is None:
        return jsonify({'code':500, 'msg': '缺少会话ID参数'})
    message_id = request.args.get('message_id')
    if message_id is None:
        return jsonify({'code':500, 'msg': '缺少消息ID参数'})
    url = "https://api.coze.cn/v1/conversation/message/retrieve?conversation_id="+conversation_id+"&message_id="+message_id
    response = pub.request(
        method='GET',
        url=url
    )
    return jsonify(response)

# @agents.route('/uploadFile', methods=['POST'])  # 文件上传测试
def upload_file(file):
    url = "https://api.coze.cn/v1/files/upload"
    response = pub.request(
        method='POST',
        url=url,
        files={'file': file}
    )
    # print(response)
    if response['code'] == 0:
        return response['data']['id']
    else:
        return None