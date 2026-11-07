package database

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSplitStatements(t *testing.T) {
	cases := []struct {
		engine string
		script string
		want   []string
	}{
		{"postgres", "SELECT 1; SELECT 2;", []string{"SELECT 1", "SELECT 2"}},
		{"postgres", `SELECT 'a;b'; SELECT "x;y" FROM t`, []string{"SELECT 'a;b'", `SELECT "x;y" FROM t`}},
		{"postgres", "-- comment;\nSELECT 1; /* ; */ ;", []string{"-- comment;\nSELECT 1"}},
		{"postgres", "CREATE FUNCTION f() RETURNS int AS $body$ SELECT 1; $body$ LANGUAGE sql; SELECT f()", []string{"CREATE FUNCTION f() RETURNS int AS $body$ SELECT 1; $body$ LANGUAGE sql", "SELECT f()"}},
		{"postgres", "DO $$ BEGIN PERFORM 1; END $$;", []string{"DO $$ BEGIN PERFORM 1; END $$"}},
		{"postgres", "SELECT 'it''s; fine'", []string{"SELECT 'it''s; fine'"}},
		{"mysql", "SELECT 'a\\';b'; # note;\nSELECT `c;d`", []string{"SELECT 'a\\';b'", "# note;\nSELECT `c;d`"}},
		{"postgres", "   ;  ; -- only comments", []string{}},
		{"postgres", "SELECT $1", []string{"SELECT $1"}},
	}
	for _, tc := range cases {
		got := SplitStatements(tc.script, tc.engine)
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%q: got %#v want %#v", tc.script, got, tc.want)
		}
	}
}

func TestReturnsRows(t *testing.T) {
	for statement, want := range map[string]bool{
		"SELECT 1":                                 true,
		"  -- x\n  select * from t":                true,
		"(SELECT 1) UNION (SELECT 2)":              true,
		"WITH x AS (SELECT 1) SELECT * FROM x":     true,
		"/* c */ SHOW TABLES":                      true,
		"EXPLAIN ANALYZE SELECT 1":                 true,
		"UPDATE t SET a = 1":                       false,
		"INSERT INTO t VALUES (1) RETURNING id":    true,
		"DELETE FROM t":                            false,
		"CREATE TABLE selected_rows (id int)":      false,
		"ALTER TABLE t ADD COLUMN returning_x int": false,
	} {
		if got := returnsRows(statement); got != want {
			t.Fatalf("%q: got %v", statement, got)
		}
	}
}

func TestValidateClient(t *testing.T) {
	good := ClientRequest{Engine: "postgres", Host: "172.18.0.2", Port: 5432, Database: "jumyskop-db", Table: "Users", OrderBy: "createdAt",
		Filters: []Filter{{Column: "email", Op: "LIKE", Value: "%@x"}}}
	if err := ValidateClient(good); err != nil {
		t.Fatal(err)
	}
	mutations := []func(*ClientRequest){
		func(r *ClientRequest) { r.Engine = "sqlite" },
		func(r *ClientRequest) { r.Host = "" },
		func(r *ClientRequest) { r.Database = "app; DROP TABLE x" },
		func(r *ClientRequest) { r.Table = "bad\x00name" },
		func(r *ClientRequest) { r.Filters[0].Op = "; DROP" },
		func(r *ClientRequest) { r.Filters[0].Column = "" },
		func(r *ClientRequest) { r.Limit = 501 },
		func(r *ClientRequest) { r.Offset = -1 },
		func(r *ClientRequest) { r.Action = "truncate" },
		func(r *ClientRequest) { r.Values = map[string]any{"a\nb": 1} },
		func(r *ClientRequest) { r.SQL = strings.Repeat("x", maxSQLBytes+1) },
	}
	for i, mutate := range mutations {
		r := good
		r.Filters = []Filter{good.Filters[0]}
		mutate(&r)
		if ValidateClient(r) == nil {
			t.Fatalf("mutation %d accepted", i)
		}
	}
}

func TestBuildWhereQuotesIdentifiers(t *testing.T) {
	known := map[string]bool{`na"me`: true, "id": true}
	pg := &sqlSession{engine: "postgres", schema: "public"}
	where, args, err := pg.buildWhere([]Filter{{Column: `na"me`, Op: "ILIKE", Value: "%a%"}, {Column: "id", Op: "!=", Value: "5"}, {Column: "id", Op: "IS NOT NULL"}}, known)
	if err != nil {
		t.Fatal(err)
	}
	if where != ` WHERE CAST("na""me" AS TEXT) ILIKE $1 AND "id" <> $2 AND "id" IS NOT NULL` || len(args) != 2 {
		t.Fatalf("unexpected: %s %v", where, args)
	}
	my := &sqlSession{engine: "mysql"}
	where, _, err = my.buildWhere([]Filter{{Column: "id", Op: "ILIKE", Value: "1"}}, known)
	if err != nil || where != " WHERE `id` LIKE ?" {
		t.Fatalf("unexpected mysql: %s %v", where, err)
	}
	if _, _, err = pg.buildWhere([]Filter{{Column: "missing", Op: "=", Value: "1"}}, known); err == nil {
		t.Fatal("unknown column accepted")
	}
	if got := my.quote("a`b"); got != "`a``b`" {
		t.Fatalf("mysql quote: %s", got)
	}
	if got := pg.qualified(`x"y`); got != `"public"."x""y"` {
		t.Fatalf("postgres qualified: %s", got)
	}
}

func TestJSONValue(t *testing.T) {
	if got := jsonValue([]byte{0xff, 0x00}); got != `\xff00` {
		t.Fatalf("binary: %v", got)
	}
	if got := jsonValue(int64(1) << 60); got != "1152921504606846976" {
		t.Fatalf("big int: %v", got)
	}
	if got := jsonValue(time.Date(2026, 11, 7, 5, 0, 0, 0, time.UTC)); got != "2026-11-07 05:00:00Z" {
		t.Fatalf("time: %v", got)
	}
}

// Optional integration check against a disposable PostgreSQL or MySQL server:
// DB_CLIENT_TEST_ENGINE, DB_CLIENT_TEST_HOST and DB_CLIENT_TEST_PASSWORD.
func TestClientIntegration(t *testing.T) {
	engine := os.Getenv("DB_CLIENT_TEST_ENGINE")
	if engine == "" {
		t.Skip("isolated database not configured")
	}
	port, user, quote := 5432, "postgres", `"`
	if engine == "mysql" {
		port, user, quote = 3306, "root", "`"
	}
	base := ClientRequest{Engine: engine, Host: os.Getenv("DB_CLIENT_TEST_HOST"), Port: port, Database: "fixture", Username: user, Password: os.Getenv("DB_CLIENT_TEST_PASSWORD")}
	ctx := context.Background()
	run := func(op string, r ClientRequest) any {
		t.Helper()
		result, err := RunClient(ctx, op, r)
		if err != nil {
			t.Fatalf("%s: %v", op, err)
		}
		return result
	}
	child := quote + "Child items" + quote
	script := base
	script.SQL = "DROP TABLE IF EXISTS " + child + "; DROP TABLE IF EXISTS records;\n" +
		"CREATE TABLE records (id integer PRIMARY KEY, title varchar(40) NOT NULL, note text);\n" +
		"CREATE TABLE " + child + " (id integer PRIMARY KEY, record_id integer, FOREIGN KEY (record_id) REFERENCES records(id));\n" +
		"INSERT INTO records VALUES (1, 'one;two', NULL), (2, 'beta', 'x'), (3, 'gamma', 'y');\n" +
		"SELECT title FROM records ORDER BY id"
	query := run("query", script).(QueryResult)
	last := query.Results[len(query.Results)-1]
	if last.Error != "" || len(last.Rows) != 3 || last.Rows[0][0] != "one;two" {
		t.Fatalf("script: %+v", query.Results)
	}
	if query.Results[4].Affected != 3 {
		t.Fatalf("insert affected: %+v", query.Results[4])
	}
	overview := run("overview", base).(Overview)
	if len(overview.Tables) != 2 || overview.Version == "" {
		t.Fatalf("overview: %+v", overview)
	}
	table := base
	table.Table = "records"
	structure := run("structure", table).(Structure)
	if len(structure.Columns) != 3 || !structure.Columns[0].Primary || structure.DDL == "" {
		t.Fatalf("structure: %+v", structure)
	}
	childTable := base
	childTable.Table = "Child items"
	if fk := run("structure", childTable).(Structure).ForeignKeys; len(fk) != 1 || fk[0].RefTable != "records" {
		t.Fatalf("foreign keys: %+v", fk)
	}
	sel := table
	sel.Filters = []Filter{{Column: "title", Op: "LIKE", Value: "%a"}}
	sel.OrderBy, sel.Desc, sel.Limit = "id", true, 1
	page := run("select", sel).(ResultSet)
	if page.Total != 2 || len(page.Rows) != 1 || page.PrimaryKey[0] != "id" {
		t.Fatalf("select: %+v", page)
	}
	insert := table
	insert.Action, insert.Values = "insert", map[string]any{"id": "4", "title": "delta", "note": nil}
	if run("rows", insert).(RowsResult).Affected != 1 {
		t.Fatal("insert failed")
	}
	update := table
	update.Action, update.Keys, update.Values = "update", []map[string]any{{"id": float64(4)}}, map[string]any{"title": "DELTA"}
	if run("rows", update).(RowsResult).Affected != 1 {
		t.Fatal("update failed")
	}
	remove := table
	remove.Action, remove.Keys = "delete", []map[string]any{{"id": "1"}, {"id": 4.0}}
	if run("rows", remove).(RowsResult).Affected != 2 {
		t.Fatal("delete failed")
	}
	duplicate := table
	duplicate.Action, duplicate.Values = "insert", map[string]any{"id": "2", "title": "dup"}
	var statementErr StatementError
	if _, err := RunClient(ctx, "rows", duplicate); !errors.As(err, &statementErr) {
		t.Fatalf("expected statement error, got %v", err)
	}
	broken := base
	broken.SQL = "SELECT 1; SELEC broken; SELECT 2"
	results := run("query", broken).(QueryResult).Results
	if len(results) != 2 || results[1].Error == "" {
		t.Fatalf("error handling: %+v", results)
	}
}
