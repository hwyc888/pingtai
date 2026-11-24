package database

import (
	"context"
	"path/filepath"
	"testing"
)

func TestKnowledgeDocumentFileMigrationLinksOriginalFiles(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "pingtai.db"))
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(context.Background(), db); err != nil { t.Fatal(err) }
	if _, err := db.ExecContext(context.Background(), "INSERT INTO ai_user (username,password,usertype,status,deltag,created_at,updated_at) VALUES ('alice','hash',0,1,0,'now','now')"); err != nil { t.Fatal(err) }
	if _, err := db.ExecContext(context.Background(), "INSERT INTO ai_identity (id,provider,subject,username,display_name,local_user_id,created_at,updated_at) VALUES ('local-alice','local','alice','alice','Alice',1,'now','now')"); err != nil { t.Fatal(err) }
	if _, err := db.ExecContext(context.Background(), "INSERT INTO ai_knowledge_base (id,name,description,enabled,created_by,created_at,updated_at) VALUES ('base-1','base','',1,'alice','now','now')"); err != nil { t.Fatal(err) }
	if _, err := db.ExecContext(context.Background(), "INSERT INTO ai_knowledge_document (id,base_id,filename,content_type,size_bytes,content_text,chunk_count,created_by,created_at,updated_at) VALUES ('doc-1','base-1','one.txt','text/plain',4,'text',1,'alice','now','now')"); err != nil { t.Fatal(err) }
	if _, err := db.ExecContext(context.Background(), "INSERT INTO platform_file (id,storage_key,original_name,sha256,owner_user_id,owner_username,created_at,updated_at) VALUES ('file-1','file-1.txt','one.txt','hash','local-alice','alice','now','now')"); err != nil { t.Fatal(err) }
	if _, err := db.ExecContext(context.Background(), "INSERT INTO ai_knowledge_document_file (document_id,base_id,file_id,created_at) VALUES ('doc-1','base-1','file-1','now')"); err != nil { t.Fatal(err) }
	var fileID string
	if err := db.QueryRowContext(context.Background(), "SELECT file_id FROM ai_knowledge_document_file WHERE document_id='doc-1'").Scan(&fileID); err != nil || fileID != "file-1" { t.Fatalf("file link not stored: file_id=%q err=%v", fileID, err) }
	if _, err := db.ExecContext(context.Background(), "DELETE FROM ai_knowledge_document WHERE id='doc-1'"); err != nil { t.Fatal(err) }
	var remaining int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM platform_file WHERE id='file-1'").Scan(&remaining); err != nil || remaining != 1 { t.Fatalf("removing knowledge document should keep original file: remaining=%d err=%v", remaining, err) }
}
