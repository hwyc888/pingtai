from flask import Blueprint, request, jsonify, current_app
from .. import jwt_blocklist  # 导入黑名单集合
from functools import wraps
from cozepy import COZE_COM_BASE_URL
from flask_jwt_extended import create_access_token, jwt_required, get_jwt_identity, create_refresh_token, get_jwt
from .. import pub
from ..models.user import User, db
import base64

datasets = Blueprint('datasets', __name__)

def allowed_file(format_type):
    """检查文件类型是否允许"""
    if format_type not in ['txt', 'pdf', 'doc', 'docx']:
        return False
    return True

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

# def get_coze_header():
#     data_dict = {"alg":"RS256","typ":"JWT","kid":"zpHVvqdIL0DWXsUpv9KJL6SsK4_IiqEg3LikgK3hvTM"}
#     json_str = json.dumps(data_dict)
#     json_bytes = json_str.encode('utf-8')
#     encoded_data = base64.urlsafe_b64encode(json_bytes).rstrip(b'=')
#     return encoded_data.decode('utf-8')

# def get_coze_payload(session_name=""):
#     data_dict = {"iss":"1186114716098",   # 应用ID
#                  "aud":"api.coze.cn",
#                  "iat":int(time.time()),   # JWT 开始生效的时间，Unixtime 时间戳格式，精确到秒
#                  "exp":int(time.time())+7200,  # JWT 失效的时间，Unixtime 时间戳格式，精确到秒
#                  "session_name":session_name,
#                  "jti":secrets.token_hex(16)}
#     json_str = json.dumps(data_dict)
#     json_bytes = json_str.encode('utf-8')
#     encoded_data = base64.urlsafe_b64encode(json_bytes).rstrip(b'=')
#     return encoded_data.decode('utf-8')


@datasets.route('/workspacesList', methods=['GET'])
@jwt_required()
@admin_required
def workspaces_list():
    url = 'https://api.coze.cn/v1/workspaces'
    response = pub.request(
        method='GET',
        url=url
    )
    data = {}
    data['data'] = {}
    data['code'] = response['code']
    data['msg'] = response['msg']
    data['data']['total_count'] = 1
    data['data']['workspaces'] = []
    for item in response['data']['workspaces']:
        print(item['id'])
        if item['id'] == "7481824441182617636":
            data['data']['workspaces'].append(item)
    return jsonify(data)
    # return jsonify(response)

@datasets.route('/createKnowledge', methods=['POST'])
@jwt_required()
@admin_required
def create_knowledge():
    url = 'https://api.coze.cn/v1/datasets'
    data = {
        'space_id': request.json.get('space_id'),
        'format_type': int(request.json.get('format_type')),
        'name': request.json.get('name'),
        'description': request.json.get('description'),
        'file_id': request.json.get('file_id'),
    }
    response = pub.request(
        method='POST',
        url=url,
        data=data
    )    
    return jsonify(response)

@datasets.route('/knowledgeList', methods=['GET'])
@jwt_required()
@admin_required
def knowledge_list():
    space_id = request.args.get('space_id')
    name = request.args.get('name')
    format_type = request.args.get('format_type')
    page_num = request.args.get('page_num', '1')
    page_size = request.args.get('page_size', '10')
    
    if space_id is None:
        return jsonify({'code':500, 'msg': '缺少空间ID参数'})
    # 构建URL查询参数
    url = f'https://api.coze.cn/v1/datasets?space_id={space_id}'
    if name:
        url += f'&name={name}'
    if format_type:
        url += f'&format_type={format_type}'
    if page_num:
        url += f'&page_num={page_num}'
    if page_size:
        url += f'&page_size={page_size}'
    response = pub.request(
        method='GET',
        url=url
    )
    return jsonify(response)

@datasets.route('/delKnowledge', methods=['POST'])
@jwt_required()
@admin_required
def get_knowledge():
    knowledge_id = request.json.get('dataset_id')
    if knowledge_id is None:
        return jsonify({'code':500, 'msg': '缺少知识库ID参数'})
    url = f'https://api.coze.cn/v1/datasets/{knowledge_id}'    
    response = pub.request(
        method='DELETE',
        url=url
    )    
    return jsonify(response)

@datasets.route('/editKnowledge', methods=['POST'])
@jwt_required()
@admin_required
def edit_knowledge():    
    knowledge_id = request.json.get('dataset_id')
    if knowledge_id is None:
        return jsonify({'code':500, 'msg': '缺少知识库ID参数'})
    url = f'https://api.coze.cn/v1/datasets/{knowledge_id}'    
    data = {
        'name': request.json.get('name'),
        'description': request.json.get('description'),
        'file_id': request.json.get('file_id'),
    }
    response = pub.request(
        method='PUT',
        url=url,
        data=data
    )    
    return jsonify(response)

# 查看知识库文件列表
@datasets.route('/knowledgeFileList', methods=['POST'])
@jwt_required()
@admin_required
def knowledge_file_list():
    dataset_id = request.json.get('dataset_id')
    page_num = request.json.get('page_num', '1')
    page_size = request.json.get('page_size', '10')
    if dataset_id is None:
        return jsonify({'code':500, 'msg': '缺少知识库ID参数'})
    url = f'https://api.coze.cn/open_api/knowledge/document/list'
    headers = {
        'Agw-Js-Conv': 'str'
    }
    data = {
        'dataset_id': dataset_id,
        'page': page_num,
        'size': page_size,
    }
    response = pub.request(
        method='POST',
        url=url,
        data=data,
        headers=headers
    )
    return jsonify(response)

# 创建知识库文件
@datasets.route('/createKnowledgeFile', methods=['POST'])
@jwt_required()
@admin_required
def create_knowledge_file():
    dataset_id = request.form.get('dataset_id')
    format_type = int(request.form.get('format_type', 0))
    if dataset_id is None:
        return jsonify({'code':500, 'msg': '缺少知识库ID参数'})
  
    url = f'https://api.coze.cn/open_api/knowledge/document/create'
    headers = {
        'Agw-Js-Conv': 'str'
    }
    
    document_bases = []
    if format_type == 0:  # 文本类型知识库
        source = request.form.get('source')
        if source is None:
            return jsonify({'code':500, 'msg': '缺少来源参数'})
        chunk_strategy = {            
                    "separator": "\n\n",
                    "max_tokens": 800,
                    "remove_extra_spaces": False,
                    "remove_urls_emails": False,
                    "chunk_type": 0  # 0系统默认的分块方式，1为自定义分块
                }
        
        if source == 'file':   # 本地文件
            if 'files' not in request.files:
                return jsonify({'code':500, 'msg': '没有上传文件'})
            
            files = request.files.getlist('files')
            if not files:
                return jsonify({'code':500, 'msg': '文件列表为空'})
            
            # 检查所有文件类型
            for file in files:
                if file.filename == '':
                    continue
                file_type = file.filename.rsplit('.', 1)[1].lower()
                if not allowed_file(file_type):
                    return jsonify({
                        'code': 400,
                        'msg': f'不支持的文件类型。'
                    })
            for file in files:
                if file.filename == '':
                    continue
                try:
                    file_type = file.filename.rsplit('.', 1)[1].lower()
                    content = file.read()
                    content =  base64.b64encode(content).decode('utf-8')
                    document_bases.append({
                        'source_info': {
                            "file_base64": content,
                            "file_type": file_type,
                        },
                        'name': file.filename,
                    })     
                    # print(document_bases)               
                except UnicodeDecodeError:
                    return jsonify({
                        'code': 500,
                        'msg': f'文件 {file.filename} 不是有效的文本文件'
                    })
            if not document_bases:
                return jsonify({'code':500, 'msg': '没有有效的文件内容'})
            
        elif source == 'url':   # 网页链接
            web_url = request.form.get('web_url')
            name = request.form.get('name')
            update_type = request.form.get('update_type', 0)
            update_interval = request.form.get('update_interval', 24)
            try:
                document_bases.append(
                    {
                        "name": name,
                        "source_info":{
                            'web_url': web_url,
                            "document_source" : 1 
                        },
                        "update_rule": {
                            "update_type": int(update_type), 
                            "update_interval": int(update_interval)
                        }
                    })
            except Exception as e:
                return jsonify({
                    'code': 500,
                    'msg': f'处理网页时出错: {str(e)}'
                })

    elif format_type == 2:  # 图片类型知识库
        document_bases.append({
            'source_info': {
                "file_base64": base64.b64encode(content).decode('utf-8'),
                "file_type": file_type,
            },
            'name': file.filename,
        })
        chunk_strategy = {
            "caption_type": 0
        }

    data = {
        'dataset_id': dataset_id,
        'format_type': format_type,
        'document_bases': document_bases,
        'chunk_strategy': chunk_strategy
    }
    
    response = pub.request(
        method='POST',
        url=url,
        data=data,
        headers=headers
    )
    return jsonify(response)

@datasets.route('/deleteKnowledgeFile', methods=['POST'])
@jwt_required()
@admin_required
def delete_knowledge_file():
    document_ids = request.json.get('document_ids')
    if document_ids is None:
        return jsonify({'code':500, 'msg': '缺少文件ID参数'})    
    url = f'https://api.coze.cn/open_api/knowledge/document/delete'
    headers = {
        'Agw-Js-Conv': 'str'
    }    
    data = {
        'document_ids': document_ids
    }    
    response = pub.request(
        method='POST',
        url=url,
        data=data,
        headers=headers
    )
    return jsonify(response)

# 查看知识库文件上传进度
@datasets.route('/getKnowledgeFileProcess', methods=['POST'])
@jwt_required()
@admin_required
def get_knowledge_file_process():
    dataset_id = request.json.get('dataset_id')
    if dataset_id is None:
        return jsonify({'code':500, 'msg': '缺少知识库ID参数'})
    document_ids = request.json.get('document_ids')
    if document_ids is None:
        return jsonify({'code':500, 'msg': '缺少文件ID参数'})

    url = f'https://api.coze.cn/v1/datasets/{dataset_id}/process'
    data = {
        'document_ids':document_ids
    }    
    response = pub.request(
        method='POST',
        url=url,
        data=data
    )
    return jsonify(response)

# 修改知识库文件
@datasets.route('/updateKnowledgeFile', methods=['POST'])
@jwt_required()
@admin_required
def update_knowledge_file():
    document_id = request.json.get('document_id')
    if document_id is None:
        return jsonify({'code':500, 'msg': '缺少文件ID参数'})
    document_name = request.json.get('document_name')
    url = f'https://api.coze.cn/open_api/knowledge/document/update'
    headers = {
        'Agw-Js-Conv': 'str'
    }
    data = {
        'document_id': document_id
    }
    if document_name:
        data['document_name'] = document_name

    update_type = request.json.get('update_type')
    update_interval = request.json.get('update_interval',24)
    if update_type is not None:
        update_rule = {
            'update_type': int(update_type),
            'update_interval': int(update_interval)
        }
        data['update_rule'] = update_rule
    
    response = pub.request(
        method='POST',
        url=url,
        data=data,
        headers=headers
    )
    return jsonify(response)

