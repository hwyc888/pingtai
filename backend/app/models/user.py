from datetime import datetime
from app import db

class User(db.Model):
    __tablename__ = 'ai_user'
    id = db.Column(db.Integer, primary_key=True)
    username = db.Column(db.String(50), unique=True)
    password = db.Column(db.String(150), unique=True)
    usertype = db.Column(db.Integer, unique=True)
    status = db.Column(db.Integer)
    deltag = db.Column(db.Integer)   # 删除标记 0未删除，1已删除
    openid = db.Column(db.String(150))
    bind_time = db.Column(db.DateTime)
    created_at = db.Column(db.DateTime)
    updated_at = db.Column(db.DateTime)