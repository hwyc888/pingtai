package config

import "testing"

func TestNormalizeMySQLURL(t *testing.T) {
	got, err := normalizeMySQLDSN("mysql+pymysql://app:secret@127.0.0.1:3306/pingtai?charset=utf8mb4")
	if err != nil {
		t.Fatal(err)
	}
	want := "app:secret@tcp(127.0.0.1:3306)/pingtai?charset=utf8mb4&loc=Local&parseTime=true"
	if got != want {
		t.Fatalf("unexpected DSN:\nwant %s\ngot  %s", want, got)
	}
}
