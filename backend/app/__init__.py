from flask import Flask, jsonify, request, send_from_directory
from flask_sqlalchemy import SQLAlchemy
from flask_jwt_extended import JWTManager
from flask_cors import CORS
from config import Config
import json
import time
import os
import requests  # 添加 requests 导入
from dotenv import load_dotenv

db = SQLAlchemy()
jwt = JWTManager()

# 创建一个简单的内存黑名单集合
jwt_blocklist = set()
load_dotenv()
class pub:
    def get_coze_token():
        token_cache_path = os.path.join(os.path.dirname(__file__), 'coze_token_cache.json')
        # 如果缓存存在且未过期，直接返回缓存
        if os.path.exists(token_cache_path):
            with open(token_cache_path, 'r') as f:
                cached_token = json.load(f)
                if int(time.time()) < int(cached_token['expires_in']) - 60:                    
                    return cached_token['token_type'] + " " + cached_token['access_token'].strip()

        coze_api_base = os.getenv('COZE_BASE_URL')
        jwt_oauth_client_id = os.getenv('COZE_APP_ID')
        jwt_oauth_private_key = ''
        jwt_oauth_private_key_file_path = os.path.join(os.path.dirname(__file__), 'private_key.pem')
        jwt_oauth_public_key_id = os.getenv('COZE_PUBLIC_KEY')
        
        # 读取私钥
        if jwt_oauth_private_key_file_path:
            try:
                # 先尝试以文本模式读取
                with open(jwt_oauth_private_key_file_path, "r", encoding='utf-8') as f:
                    jwt_oauth_private_key = f.read().strip()
                    # print("Private key content:", jwt_oauth_private_key[:50], "...")  # 打印私钥前50个字符
                    # print("Private key type:", type(jwt_oauth_private_key))
                    
                    # 确保私钥格式正确
                    if not jwt_oauth_private_key.startswith('-----BEGIN PRIVATE KEY-----'):
                        # print("Warning: Private key does not have expected format")
                        # 如果格式不正确，尝试二进制模式读取
                        with open(jwt_oauth_private_key_file_path, "rb") as f:
                            jwt_oauth_private_key = f.read()
                            # 尝试解码为字符串
                            if isinstance(jwt_oauth_private_key, bytes):
                                jwt_oauth_private_key = jwt_oauth_private_key.decode('utf-8').strip()
                                # print("Converted private key:", jwt_oauth_private_key[:50], "...")
            except Exception as e:
                # print(f"Error reading private key: {str(e)}")
                # print(f"File path: {jwt_oauth_private_key_file_path}")
                # print(f"File exists: {os.path.exists(jwt_oauth_private_key_file_path)}")
                raise

        from cozepy import JWTOAuthApp

        # print("Client ID:", jwt_oauth_client_id)
        # print("Public Key ID:", jwt_oauth_public_key_id)
        # print("Base URL:", coze_api_base)

        jwt_oauth_app = JWTOAuthApp(
            client_id=jwt_oauth_client_id,
            private_key=jwt_oauth_private_key,
            public_key_id=jwt_oauth_public_key_id,
            base_url=coze_api_base,
        )

        try:
            # print("Getting access token...")
            oauth_token = jwt_oauth_app.get_access_token(ttl=7200)
            # print("Got access token:", oauth_token)

            # 将 OAuthToken 对象转换为字典
            token_dict = {
                'access_token': oauth_token.access_token,
                'expires_in': oauth_token.expires_in,  # 存储过期时间戳
                'refresh_token': oauth_token.refresh_token,
                'token_type': oauth_token.token_type
            }
            
            # 缓存token到本地
            with open(token_cache_path, 'w') as f:
                json.dump(token_dict, f)
            
            return token_dict['token_type'] + " " + token_dict['access_token']
        except Exception as e:
            # print(f"Error getting token: {str(e)}")
            # print(f"Error type: {type(e)}")
            # import traceback
            # print(f"Stack trace:\n{traceback.format_exc()}")
            raise
    
    @staticmethod
    def request(method, url, data=None, headers=None, files=None):
        """
        发起远程请求的通用函数
        
        Args:
            method (str): 请求方法 ('GET', 'POST', 'PUT', 'DELETE' 等)
            url (str): 请求地址
            data (dict, optional): 请求数据，默认为 None
            headers (dict, optional): 请求头，默认为 None
            files (dict, optional): 文件数据，默认为 None
            
        Returns:
            dict: 响应的 JSON 数据
            
        Raises:
            requests.exceptions.RequestException: 当请求失败时抛出异常
        """
        try:
            if headers is None:
                headers = {}
            # 如果需要 Coze token，自动添加到 headers
            if 'api.coze.cn' in url and 'Authorization' not in headers:
                headers['Authorization'] = pub.get_coze_token()

            # 处理文件上传的情况
            if files:
                # 文件上传时不设置 Content-Type，让 requests 自动处理
                if 'Content-Type' in headers:
                    del headers['Content-Type']
            else:
                # 非文件上传时设置默认的 Content-Type
                if 'Content-Type' not in headers:
                    headers['Content-Type'] = 'application/json'
                # JSON序列化请求数据
                if data and headers.get('Content-Type') == 'application/json':
                    data = json.dumps(data)
            response = requests.request(
                method=method.upper(),
                url=url,
                data=data,
                files=files,
                headers=headers,
                timeout=30
            )
            
            # 确保请求成功
            response.raise_for_status()
            try:
                return response.json()
            except ValueError:
                return {'data': response.text}
                
        except requests.exceptions.RequestException as e:
            error_msg = str(e)
            if hasattr(e.response, 'json'):
                try:
                    error_msg = e.response.json()
                except ValueError:
                    error_msg = e.response.text
            return {
                'code': e.response.status_code if hasattr(e, 'response') else 500,
                'msg': error_msg,
                'error': True
            }

def create_app(config_class=Config):
    app = Flask(__name__)
    app.config.from_object(config_class)
    
    # 添加JWT配置
    app.config['JWT_TOKEN_LOCATION'] = ['headers']
    app.config['JWT_HEADER_NAME'] = 'Authorization'
    app.config['JWT_HEADER_TYPE'] = 'Bearer'
    app.config['SERVER_URL'] = 'http://localhost:5050'
    
    # 添加 Coze 配置
    app.config['COZE_BASE_URL'] = os.environ.get('COZE_BASE_URL', 'https://api.coze.cn')
    app.config['COZE_APP_ID'] = os.environ.get('COZE_APP_ID')
    app.config['COZE_PRIVATE_KEY'] = os.environ.get('COZE_PRIVATE_KEY')
    
    # 允许跨域
    CORS(app)
    
    db.init_app(app)
    jwt.init_app(app)

    # 注册JWT黑名单检查回调
    @jwt.token_in_blocklist_loader
    def check_if_token_in_blocklist(jwt_header, jwt_payload):
        return jwt_payload["jti"] in jwt_blocklist
    
    @jwt.unauthorized_loader
    def unauthorized_callback(callback):
        return jsonify({
            'code': 401,
            'msg': 'Missing Authorization Header'
        }), 401
        
    @jwt.invalid_token_loader
    def invalid_token_callback(callback):
        return jsonify({
            'code': 401,
            'msg': 'Invalid token'
        }), 401
    
    from app.routes.auth import auth
    app.register_blueprint(auth, url_prefix='/auth')

    from app.routes.datasets import datasets
    app.register_blueprint(datasets, url_prefix='/datasets')

    from app.routes.agents import agents
    app.register_blueprint(agents, url_prefix='/agents')

    return app 