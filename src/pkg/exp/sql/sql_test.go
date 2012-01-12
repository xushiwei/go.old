// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sql

import (
	"reflect"
	"strings"
	"testing"
)

func newTestDB(t *testing.T, name string) *DB {
	db, err := Open("test", "foo")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := db.Exec("WIPE"); err != nil {
		t.Fatalf("exec wipe: %v", err)
	}
	if name == "people" {
		exec(t, db, "CREATE|people|name=string,age=int32,dead=bool")
		exec(t, db, "INSERT|people|name=Alice,age=?", 1)
		exec(t, db, "INSERT|people|name=Bob,age=?", 2)
		exec(t, db, "INSERT|people|name=Chris,age=?", 3)
	}
	return db
}

func exec(t *testing.T, db *DB, query string, args ...interface{}) {
	_, err := db.Exec(query, args...)
	if err != nil {
		t.Fatalf("Exec of %q: %v", query, err)
	}
}

func closeDB(t *testing.T, db *DB) {
	err := db.Close()
	if err != nil {
		t.Fatalf("error closing DB: %v", err)
	}
}

func TestQuery(t *testing.T) {
	db := newTestDB(t, "people")
	defer closeDB(t, db)
	rows, err := db.Query("SELECT|people|age,name|")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	type row struct {
		age  int
		name string
	}
	got := []row{}
	for rows.Next() {
		var r row
		err = rows.Scan(&r.age, &r.name)
		if err != nil {
			t.Fatalf("Scan: %v", err)
		}
		got = append(got, r)
	}
	err = rows.Err()
	if err != nil {
		t.Fatalf("Err: %v", err)
	}
	want := []row{
		{age: 1, name: "Alice"},
		{age: 2, name: "Bob"},
		{age: 3, name: "Chris"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Logf(" got: %#v\nwant: %#v", got, want)
	}
}

func TestRowsColumns(t *testing.T) {
	db := newTestDB(t, "people")
	defer closeDB(t, db)
	rows, err := db.Query("SELECT|people|age,name|")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("Columns: %v", err)
	}
	want := []string{"age", "name"}
	if !reflect.DeepEqual(cols, want) {
		t.Errorf("got %#v; want %#v", cols, want)
	}
}

func TestQueryRow(t *testing.T) {
	db := newTestDB(t, "people")
	defer closeDB(t, db)
	var name string
	var age int

	err := db.QueryRow("SELECT|people|age,name|age=?", 3).Scan(&age)
	if err == nil || !strings.Contains(err.Error(), "expected 2 destination arguments") {
		t.Errorf("expected error from wrong number of arguments; actually got: %v", err)
	}

	err = db.QueryRow("SELECT|people|age,name|age=?", 2).Scan(&age, &name)
	if err != nil {
		t.Fatalf("age QueryRow+Scan: %v", err)
	}
	if name != "Bob" {
		t.Errorf("expected name Bob, got %q", name)
	}
	if age != 2 {
		t.Errorf("expected age 2, got %d", age)
	}

	err = db.QueryRow("SELECT|people|age,name|name=?", "Alice").Scan(&age, &name)
	if err != nil {
		t.Fatalf("name QueryRow+Scan: %v", err)
	}
	if name != "Alice" {
		t.Errorf("expected name Alice, got %q", name)
	}
	if age != 1 {
		t.Errorf("expected age 1, got %d", age)
	}
}

func TestStatementErrorAfterClose(t *testing.T) {
	db := newTestDB(t, "people")
	defer closeDB(t, db)
	stmt, err := db.Prepare("SELECT|people|age|name=?")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	err = stmt.Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	var name string
	err = stmt.QueryRow("foo").Scan(&name)
	if err == nil {
		t.Errorf("expected error from QueryRow.Scan after Stmt.Close")
	}
}

func TestStatementQueryRow(t *testing.T) {
	db := newTestDB(t, "people")
	defer closeDB(t, db)
	stmt, err := db.Prepare("SELECT|people|age|name=?")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	var age int
	for n, tt := range []struct {
		name string
		want int
	}{
		{"Alice", 1},
		{"Bob", 2},
		{"Chris", 3},
	} {
		if err := stmt.QueryRow(tt.name).Scan(&age); err != nil {
			t.Errorf("%d: on %q, QueryRow/Scan: %v", n, tt.name, err)
		} else if age != tt.want {
			t.Errorf("%d: age=%d, want %d", n, age, tt.want)
		}
	}

}

// just a test of fakedb itself
func TestBogusPreboundParameters(t *testing.T) {
	db := newTestDB(t, "foo")
	defer closeDB(t, db)
	exec(t, db, "CREATE|t1|name=string,age=int32,dead=bool")
	_, err := db.Prepare("INSERT|t1|name=?,age=bogusconversion")
	if err == nil {
		t.Fatalf("expected error")
	}
	if err.Error() != `fakedb: invalid conversion to int32 from "bogusconversion"` {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExec(t *testing.T) {
	db := newTestDB(t, "foo")
	defer closeDB(t, db)
	exec(t, db, "CREATE|t1|name=string,age=int32,dead=bool")
	stmt, err := db.Prepare("INSERT|t1|name=?,age=?")
	if err != nil {
		t.Errorf("Stmt, err = %v, %v", stmt, err)
	}

	type execTest struct {
		args    []interface{}
		wantErr string
	}
	execTests := []execTest{
		// Okay:
		{[]interface{}{"Brad", 31}, ""},
		{[]interface{}{"Brad", int64(31)}, ""},
		{[]interface{}{"Bob", "32"}, ""},
		{[]interface{}{7, 9}, ""},

		// Invalid conversions:
		{[]interface{}{"Brad", int64(0xFFFFFFFF)}, "sql: converting Exec argument #1's type: sql/driver: value 4294967295 overflows int32"},
		{[]interface{}{"Brad", "strconv fail"}, "sql: converting Exec argument #1's type: sql/driver: value \"strconv fail\" can't be converted to int32"},

		// Wrong number of args:
		{[]interface{}{}, "sql: expected 2 arguments, got 0"},
		{[]interface{}{1, 2, 3}, "sql: expected 2 arguments, got 3"},
	}
	for n, et := range execTests {
		_, err := stmt.Exec(et.args...)
		errStr := ""
		if err != nil {
			errStr = err.Error()
		}
		if errStr != et.wantErr {
			t.Errorf("stmt.Execute #%d: for %v, got error %q, want error %q",
				n, et.args, errStr, et.wantErr)
		}
	}
}

func TestTxStmt(t *testing.T) {
	db := newTestDB(t, "")
	defer closeDB(t, db)
	exec(t, db, "CREATE|t1|name=string,age=int32,dead=bool")
	stmt, err := db.Prepare("INSERT|t1|name=?,age=?")
	if err != nil {
		t.Fatalf("Stmt, err = %v, %v", stmt, err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin = %v", err)
	}
	_, err = tx.Stmt(stmt).Exec("Bobby", 7)
	if err != nil {
		t.Fatalf("Exec = %v", err)
	}
	err = tx.Commit()
	if err != nil {
		t.Fatalf("Commit = %v", err)
	}
}

// Tests fix for issue 2542, that we release a lock when querying on
// a closed connection.
func TestIssue2542Deadlock(t *testing.T) {
	db := newTestDB(t, "people")
	closeDB(t, db)
	for i := 0; i < 2; i++ {
		_, err := db.Query("SELECT|people|age,name|")
		if err == nil {
			t.Fatalf("expected error")
		}
	}
}
