package store

/*
#cgo LDFLAGS: -L/lib/x86_64-linux-gnu -l:libsqlite3.so.0
#include <stdlib.h>
#include <string.h>
typedef struct sqlite3 sqlite3;
typedef struct sqlite3_stmt sqlite3_stmt;
int sqlite3_open(const char*, sqlite3**); int sqlite3_close(sqlite3*);
int sqlite3_exec(sqlite3*,const char*,int(*)(void*,int,char**,char**),void*,char**);
void sqlite3_free(void*); const char *sqlite3_errmsg(sqlite3*);
int sqlite3_prepare_v2(sqlite3*,const char*,int,sqlite3_stmt**,const char**);
int sqlite3_bind_blob(sqlite3_stmt*,int,const void*,int,void(*)(void*));
int sqlite3_step(sqlite3_stmt*); int sqlite3_finalize(sqlite3_stmt*);
const void *sqlite3_column_blob(sqlite3_stmt*,int); int sqlite3_column_bytes(sqlite3_stmt*,int);
#define SQLITE_OK 0
#define SQLITE_ROW 100
#define SQLITE_DONE 101
#define SQLITE_TRANSIENT ((void(*)(void*))-1)
static int bz_init(const char *path,char **message){sqlite3 *db=0;if(sqlite3_open(path,&db)!=SQLITE_OK){*message=strdup(sqlite3_errmsg(db));if(db)sqlite3_close(db);return 1;}const char *sql="PRAGMA journal_mode=WAL;PRAGMA foreign_keys=ON;PRAGMA busy_timeout=5000;BEGIN;CREATE TABLE IF NOT EXISTS schema_versions(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL);INSERT OR IGNORE INTO schema_versions VALUES(1,datetime('now'));CREATE TABLE IF NOT EXISTS aggregate_state(id INTEGER PRIMARY KEY CHECK(id=1),payload BLOB NOT NULL,updated_at TEXT NOT NULL);CREATE TABLE IF NOT EXISTS test_runs(id TEXT PRIMARY KEY,revision INTEGER NOT NULL,status TEXT NOT NULL);CREATE TABLE IF NOT EXISTS data_packages(id TEXT PRIMARY KEY,test_run_id TEXT NOT NULL);CREATE TABLE IF NOT EXISTS anomalies(id TEXT PRIMARY KEY,test_run_id TEXT NOT NULL);CREATE TABLE IF NOT EXISTS disposition_evidence(id TEXT PRIMARY KEY,anomaly_id TEXT NOT NULL);CREATE TABLE IF NOT EXISTS validity_decisions(id TEXT PRIMARY KEY,test_run_id TEXT NOT NULL);CREATE TABLE IF NOT EXISTS idempotency_results(request_id TEXT PRIMARY KEY,fingerprint TEXT NOT NULL,response BLOB NOT NULL);CREATE TABLE IF NOT EXISTS audit_events(sequence INTEGER PRIMARY KEY,test_run_id TEXT NOT NULL,event_hash TEXT NOT NULL UNIQUE,previous_hash TEXT NOT NULL);COMMIT;";char *err=0;int rc=sqlite3_exec(db,sql,0,0,&err);if(rc!=SQLITE_OK){*message=strdup(err?err:sqlite3_errmsg(db));if(err)sqlite3_free(err);}sqlite3_close(db);return rc==SQLITE_OK?0:1;}
static int bz_load(const char *path,void **data,int *length,char **message){sqlite3 *db=0;sqlite3_stmt *st=0;if(sqlite3_open(path,&db)!=SQLITE_OK){*message=strdup(sqlite3_errmsg(db));return 1;}if(sqlite3_prepare_v2(db,"SELECT payload FROM aggregate_state WHERE id=1",-1,&st,0)!=SQLITE_OK){*message=strdup(sqlite3_errmsg(db));sqlite3_close(db);return 1;}int rc=sqlite3_step(st);if(rc==SQLITE_ROW){*length=sqlite3_column_bytes(st,0);*data=malloc(*length);memcpy(*data,sqlite3_column_blob(st,0),*length);}else if(rc!=SQLITE_DONE){*message=strdup(sqlite3_errmsg(db));sqlite3_finalize(st);sqlite3_close(db);return 1;}sqlite3_finalize(st);sqlite3_close(db);return 0;}
static int bz_save(const char *path,const void *data,int length,char **message){sqlite3 *db=0;sqlite3_stmt *st=0;if(sqlite3_open(path,&db)!=SQLITE_OK){*message=strdup(sqlite3_errmsg(db));return 1;}char *err=0;if(sqlite3_exec(db,"BEGIN IMMEDIATE",0,0,&err)!=SQLITE_OK){*message=strdup(err?err:sqlite3_errmsg(db));if(err)sqlite3_free(err);sqlite3_close(db);return 1;}const char *q="INSERT INTO aggregate_state(id,payload,updated_at) VALUES(1,?,datetime('now')) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload,updated_at=excluded.updated_at";if(sqlite3_prepare_v2(db,q,-1,&st,0)!=SQLITE_OK||sqlite3_bind_blob(st,1,data,length,SQLITE_TRANSIENT)!=SQLITE_OK||sqlite3_step(st)!=SQLITE_DONE){*message=strdup(sqlite3_errmsg(db));if(st)sqlite3_finalize(st);sqlite3_exec(db,"ROLLBACK",0,0,0);sqlite3_close(db);return 1;}sqlite3_finalize(st);if(sqlite3_exec(db,"COMMIT",0,0,&err)!=SQLITE_OK){*message=strdup(err?err:sqlite3_errmsg(db));if(err)sqlite3_free(err);sqlite3_close(db);return 1;}sqlite3_close(db);return 0;}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func cError(message *C.char) error {
	if message == nil {
		return fmt.Errorf("SQLite 操作失败")
	}
	defer C.free(unsafe.Pointer(message))
	return fmt.Errorf("SQLite: %s", C.GoString(message))
}
func sqliteInit(path string) error {
	p := C.CString(path)
	defer C.free(unsafe.Pointer(p))
	var msg *C.char
	if C.bz_init(p, &msg) != 0 {
		return cError(msg)
	}
	return nil
}
func sqliteLoad(path string) ([]byte, error) {
	p := C.CString(path)
	defer C.free(unsafe.Pointer(p))
	var data unsafe.Pointer
	var n C.int
	var msg *C.char
	if C.bz_load(p, &data, &n, &msg) != 0 {
		return nil, cError(msg)
	}
	if data == nil {
		return nil, nil
	}
	defer C.free(data)
	return C.GoBytes(data, n), nil
}
func sqliteSave(path string, data []byte) error {
	p := C.CString(path)
	defer C.free(unsafe.Pointer(p))
	var msg *C.char
	var ptr unsafe.Pointer
	if len(data) > 0 {
		ptr = unsafe.Pointer(&data[0])
	}
	if C.bz_save(p, ptr, C.int(len(data)), &msg) != 0 {
		return cError(msg)
	}
	return nil
}
