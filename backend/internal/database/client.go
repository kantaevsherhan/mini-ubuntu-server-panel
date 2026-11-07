package database

// Adminer-style database client: metadata, structure, filtered browsing,
// arbitrary SQL and row editing. Like the read-only browser, every request
// opens its own short-lived connection and never persists credentials.
import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	ErrNoPrimaryKey = errors.New("table has no primary key")
	ErrUnsupported  = errors.New("operation is not supported for this engine")
)

// StatementError is an error reported by the database server the administrator
// connected to (syntax error, constraint violation, ...). Its message is shown
// in the UI because it describes the administrator's own query, not the panel.
type StatementError struct{ Message string }

func (e StatementError) Error() string { return e.Message }

var clientDatabaseName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.$-]{0,127}$`)

const (
	maxQueryRows     = 1000
	maxPageRows      = 500
	defaultPageRows  = 50
	maxClientPayload = 8 << 20
	maxStatements    = 50
	maxSQLBytes      = 1 << 20
	maxKeys          = 500
)

var filterOperators = map[string]bool{
	"=": true, "!=": true, "<": true, ">": true, "<=": true, ">=": true,
	"LIKE": true, "NOT LIKE": true, "ILIKE": true, "IS NULL": true, "IS NOT NULL": true,
}

var ClientOperations = map[string]bool{"overview": true, "structure": true, "select": true, "query": true, "rows": true}

type Filter struct {
	Column string `json:"column"`
	Op     string `json:"op"`
	Value  string `json:"value"`
}

type ClientRequest struct {
	Engine     string `json:"engine"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Database   string `json:"database"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	TLS        bool   `json:"tls"`
	AuthSource string `json:"auth_source"`
	Container  string `json:"container"`
	Schema     string `json:"schema"`
	Table      string `json:"table"`

	Offset      int      `json:"offset"`
	Limit       int      `json:"limit"`
	OrderBy     string   `json:"order_by"`
	Desc        bool     `json:"desc"`
	Filters     []Filter `json:"filters"`
	MongoFilter string   `json:"mongo_filter"`

	SQL string `json:"sql"`

	Action   string           `json:"action"`
	Keys     []map[string]any `json:"keys"`
	Values   map[string]any   `json:"values"`
	Document string           `json:"document"`
}

type TableInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Rows    int64  `json:"rows"`
	Size    int64  `json:"size"`
	Comment string `json:"comment"`
}

type Overview struct {
	Version   string      `json:"version"`
	Databases []string    `json:"databases"`
	Schemas   []string    `json:"schemas"`
	Schema    string      `json:"schema"`
	Tables    []TableInfo `json:"tables"`
}

type ColumnInfo struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	Nullable      bool   `json:"nullable"`
	Default       string `json:"default"`
	Primary       bool   `json:"primary"`
	AutoIncrement bool   `json:"auto_increment"`
	Comment       string `json:"comment"`
}

type IndexInfo struct {
	Name       string `json:"name"`
	Columns    string `json:"columns"`
	Unique     bool   `json:"unique"`
	Primary    bool   `json:"primary"`
	Definition string `json:"definition"`
}

type ForeignKey struct {
	Name       string `json:"name"`
	Columns    string `json:"columns"`
	RefTable   string `json:"ref_table"`
	RefColumns string `json:"ref_columns"`
	Definition string `json:"definition"`
}

type Structure struct {
	Type        string       `json:"type"`
	Columns     []ColumnInfo `json:"columns"`
	Indexes     []IndexInfo  `json:"indexes"`
	ForeignKeys []ForeignKey `json:"foreign_keys"`
	DDL         string       `json:"ddl"`
}

type ResultSet struct {
	Statement  string   `json:"statement,omitempty"`
	Columns    []string `json:"columns"`
	Types      []string `json:"types"`
	Rows       [][]any  `json:"rows"`
	Documents  []string `json:"documents,omitempty"`
	PrimaryKey []string `json:"primary_key"`
	Total      int64    `json:"total"`
	Affected   int64    `json:"affected"`
	DurationMS int64    `json:"duration_ms"`
	Truncated  bool     `json:"truncated"`
	Error      string   `json:"error,omitempty"`
}

type QueryResult struct {
	Results []ResultSet `json:"results"`
}

type RowsResult struct {
	Affected int64 `json:"affected"`
}

func validClientIdentifier(name string) bool {
	if name == "" || len(name) > 128 || !utf8.ValidString(name) {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func ValidateClient(r ClientRequest) error {
	if r.Engine != "postgres" && r.Engine != "mysql" && r.Engine != "mongodb" {
		return ErrExternalInvalid
	}
	if !externalHost.MatchString(r.Host) || r.Port < 1 || r.Port > 65535 || !clientDatabaseName.MatchString(r.Database) {
		return ErrExternalInvalid
	}
	if len(r.Username) > 256 || len(r.Password) > 4096 || strings.ContainsAny(r.Username+r.Password, "\x00") {
		return ErrExternalInvalid
	}
	if r.AuthSource != "" && !externalName.MatchString(r.AuthSource) {
		return ErrExternalInvalid
	}
	if (r.Schema != "" && !validClientIdentifier(r.Schema)) || (r.Table != "" && !validClientIdentifier(r.Table)) {
		return ErrExternalInvalid
	}
	if r.Offset < 0 || r.Offset > 10_000_000 || r.Limit < 0 || r.Limit > maxPageRows {
		return ErrExternalInvalid
	}
	if r.OrderBy != "" && !validClientIdentifier(r.OrderBy) {
		return ErrExternalInvalid
	}
	if len(r.Filters) > 20 {
		return ErrExternalInvalid
	}
	for _, f := range r.Filters {
		if !validClientIdentifier(f.Column) || !filterOperators[f.Op] || len(f.Value) > 4096 {
			return ErrExternalInvalid
		}
	}
	if len(r.SQL) > maxSQLBytes || len(r.MongoFilter) > 65536 || len(r.Document) > maxSQLBytes || len(r.Keys) > maxKeys || len(r.Values) > 1000 {
		return ErrExternalInvalid
	}
	if r.Action != "" && r.Action != "insert" && r.Action != "update" && r.Action != "delete" {
		return ErrExternalInvalid
	}
	for column := range r.Values {
		if !validClientIdentifier(column) {
			return ErrExternalInvalid
		}
	}
	return nil
}

// RunClient executes one database client operation and returns a JSON-ready result.
func RunClient(ctx context.Context, op string, r ClientRequest) (any, error) {
	if !ClientOperations[op] {
		return nil, ErrExternalInvalid
	}
	if err := ValidateClient(r); err != nil {
		return nil, err
	}
	var (
		result any
		err    error
	)
	if r.Engine == "mongodb" {
		result, err = runMongo(ctx, op, r)
	} else {
		result, err = runSQL(ctx, op, r)
	}
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	if len(encoded) > maxClientPayload {
		return nil, ErrExternalTooLarge
	}
	return result, nil
}

type sqlSession struct {
	engine string
	pool   *sql.DB
	conn   *sql.Conn
	schema string
}

func (r ClientRequest) external() ExternalRequest {
	return ExternalRequest{Engine: r.Engine, Host: r.Host, Port: r.Port, Database: r.Database, Username: r.Username, Password: r.Password, TLS: r.TLS, AuthSource: r.AuthSource}
}

func runSQL(ctx context.Context, op string, r ClientRequest) (any, error) {
	timeout := 10 * time.Second
	if op == "query" {
		timeout = 28 * time.Second
	}
	s, err := openClientSQL(ctx, r, timeout)
	if err != nil {
		return nil, err
	}
	defer s.close()
	switch op {
	case "overview":
		return s.overview(ctx)
	case "structure":
		if r.Table == "" {
			return nil, ErrExternalInvalid
		}
		return s.structure(ctx, r.Table)
	case "select":
		if r.Table == "" {
			return nil, ErrExternalInvalid
		}
		return s.selectRows(ctx, r)
	case "query":
		return s.query(ctx, r.SQL)
	default:
		if r.Table == "" {
			return nil, ErrExternalInvalid
		}
		return s.modifyRows(ctx, r)
	}
}

func openClientSQL(ctx context.Context, r ClientRequest, timeout time.Duration) (*sqlSession, error) {
	dialector := sqlDialector(r.external(), timeout)
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
	if err != nil {
		return nil, err
	}
	pool, err := db.DB()
	if err != nil {
		return nil, err
	}
	pool.SetMaxOpenConns(1)
	conn, err := pool.Conn(ctx)
	if err != nil {
		_ = pool.Close()
		return nil, err
	}
	s := &sqlSession{engine: r.Engine, pool: pool, conn: conn, schema: r.Schema}
	if s.engine == "postgres" && s.schema == "" {
		if err = conn.QueryRowContext(ctx, "SELECT current_schema()").Scan(&s.schema); err != nil {
			s.close()
			return nil, err
		}
	}
	return s, nil
}

func (s *sqlSession) close() {
	_ = s.conn.Close()
	_ = s.pool.Close()
}

func (s *sqlSession) quote(name string) string {
	if s.engine == "mysql" {
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (s *sqlSession) qualified(table string) string {
	if s.engine == "postgres" {
		return s.quote(s.schema) + "." + s.quote(table)
	}
	return s.quote(table)
}

func (s *sqlSession) placeholder(n int) string {
	if s.engine == "mysql" {
		return "?"
	}
	return "$" + strconv.Itoa(n)
}

func statementError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		message := pgErr.Message
		if pgErr.Detail != "" {
			message += " — " + pgErr.Detail
		}
		if pgErr.Hint != "" {
			message += " (" + pgErr.Hint + ")"
		}
		return StatementError{Message: truncateMessage("ERROR " + pgErr.Code + ": " + message)}
	}
	var myErr *mysqldriver.MySQLError
	if errors.As(err, &myErr) {
		return StatementError{Message: truncateMessage(fmt.Sprintf("ERROR %d: %s", myErr.Number, myErr.Message))}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return StatementError{Message: "query timed out"}
	}
	if errors.Is(err, ErrExternalTooLarge) || errors.Is(err, ErrExternalInvalid) || errors.Is(err, ErrNoPrimaryKey) {
		return err
	}
	return StatementError{Message: "query failed"}
}

func truncateMessage(message string) string {
	if len(message) > 2000 {
		return message[:2000] + "…"
	}
	return message
}

func (s *sqlSession) listStrings(ctx context.Context, query string, args ...any) ([]string, error) {
	rows, err := s.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []string{}
	for rows.Next() {
		var value string
		if err = rows.Scan(&value); err != nil {
			return nil, err
		}
		items = append(items, value)
	}
	return items, rows.Err()
}

func (s *sqlSession) overview(ctx context.Context) (Overview, error) {
	result := Overview{Databases: []string{}, Schemas: []string{}, Tables: []TableInfo{}, Schema: s.schema}
	var tableQuery string
	var args []any
	if s.engine == "postgres" {
		_ = s.conn.QueryRowContext(ctx, "SELECT version()").Scan(&result.Version)
		result.Databases, _ = s.listStrings(ctx, "SELECT datname FROM pg_database WHERE NOT datistemplate AND datallowconn ORDER BY 1")
		schemas, err := s.listStrings(ctx, `SELECT nspname FROM pg_namespace WHERE nspname NOT LIKE 'pg\_%' AND nspname <> 'information_schema' ORDER BY nspname = 'public' DESC, 1`)
		if err != nil {
			return result, statementError(err)
		}
		result.Schemas = schemas
		tableQuery = `SELECT c.relname,
  CASE c.relkind WHEN 'v' THEN 'view' WHEN 'm' THEN 'materialized view' WHEN 'f' THEN 'foreign table' ELSE 'table' END,
  GREATEST(c.reltuples, 0)::bigint, pg_total_relation_size(c.oid), COALESCE(obj_description(c.oid, 'pg_class'), '')
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = $1 AND c.relkind IN ('r', 'p', 'v', 'm', 'f')
ORDER BY c.relname`
		args = []any{s.schema}
	} else {
		_ = s.conn.QueryRowContext(ctx, "SELECT VERSION()").Scan(&result.Version)
		result.Databases, _ = s.listStrings(ctx, "SHOW DATABASES")
		tableQuery = `SELECT TABLE_NAME, CASE TABLE_TYPE WHEN 'VIEW' THEN 'view' WHEN 'SYSTEM VIEW' THEN 'view' ELSE 'table' END,
  COALESCE(TABLE_ROWS, 0), COALESCE(DATA_LENGTH, 0) + COALESCE(INDEX_LENGTH, 0), COALESCE(TABLE_COMMENT, '')
FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() ORDER BY TABLE_NAME`
	}
	rows, err := s.conn.QueryContext(ctx, tableQuery, args...)
	if err != nil {
		return result, statementError(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var table TableInfo
		if err = rows.Scan(&table.Name, &table.Type, &table.Rows, &table.Size, &table.Comment); err != nil {
			return result, statementError(err)
		}
		result.Tables = append(result.Tables, table)
	}
	return result, statementError(rows.Err())
}

func (s *sqlSession) structure(ctx context.Context, table string) (Structure, error) {
	if s.engine == "postgres" {
		return s.postgresStructure(ctx, table)
	}
	return s.mysqlStructure(ctx, table)
}

func (s *sqlSession) postgresStructure(ctx context.Context, table string) (Structure, error) {
	result := Structure{Columns: []ColumnInfo{}, Indexes: []IndexInfo{}, ForeignKeys: []ForeignKey{}}
	var oid uint32
	var kind string
	err := s.conn.QueryRowContext(ctx, `SELECT c.oid, c.relkind::text FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = $1 AND c.relname = $2 AND c.relkind IN ('r', 'p', 'v', 'm', 'f')`, s.schema, table).Scan(&oid, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrExternalInvalid
	}
	if err != nil {
		return result, statementError(err)
	}
	result.Type = map[string]string{"v": "view", "m": "materialized view", "f": "foreign table"}[kind]
	if result.Type == "" {
		result.Type = "table"
	}
	rows, err := s.conn.QueryContext(ctx, `SELECT a.attname, format_type(a.atttypid, a.atttypmod), NOT a.attnotnull,
  COALESCE(pg_get_expr(d.adbin, d.adrelid), ''),
  EXISTS (SELECT 1 FROM pg_index ix WHERE ix.indrelid = a.attrelid AND ix.indisprimary AND a.attnum = ANY(ix.indkey)),
  a.attidentity <> '' OR COALESCE(pg_get_expr(d.adbin, d.adrelid), '') LIKE 'nextval(%',
  COALESCE(col_description(a.attrelid, a.attnum), '')
FROM pg_attribute a LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
WHERE a.attrelid = $1 AND a.attnum > 0 AND NOT a.attisdropped ORDER BY a.attnum`, oid)
	if err != nil {
		return result, statementError(err)
	}
	for rows.Next() {
		var c ColumnInfo
		if err = rows.Scan(&c.Name, &c.Type, &c.Nullable, &c.Default, &c.Primary, &c.AutoIncrement, &c.Comment); err != nil {
			_ = rows.Close()
			return result, statementError(err)
		}
		result.Columns = append(result.Columns, c)
	}
	_ = rows.Close()
	rows, err = s.conn.QueryContext(ctx, `SELECT i.relname, ix.indisunique, ix.indisprimary, pg_get_indexdef(ix.indexrelid),
  array_to_string(ARRAY(SELECT pg_get_indexdef(ix.indexrelid, k + 1, true) FROM generate_subscripts(ix.indkey, 1) k ORDER BY k), ', '),
  EXISTS (SELECT 1 FROM pg_constraint co WHERE co.conindid = ix.indexrelid)
FROM pg_index ix JOIN pg_class i ON i.oid = ix.indexrelid
WHERE ix.indrelid = $1 ORDER BY ix.indisprimary DESC, i.relname`, oid)
	if err != nil {
		return result, statementError(err)
	}
	standalone := []string{}
	for rows.Next() {
		var index IndexInfo
		var constraint bool
		if err = rows.Scan(&index.Name, &index.Unique, &index.Primary, &index.Definition, &index.Columns, &constraint); err != nil {
			_ = rows.Close()
			return result, statementError(err)
		}
		if !constraint {
			standalone = append(standalone, index.Definition+";")
		}
		result.Indexes = append(result.Indexes, index)
	}
	_ = rows.Close()
	rows, err = s.conn.QueryContext(ctx, `SELECT con.conname, con.contype::text, pg_get_constraintdef(con.oid),
  array_to_string(ARRAY(SELECT a.attname FROM unnest(con.conkey) WITH ORDINALITY k(n, o) JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = k.n ORDER BY k.o), ', '),
  COALESCE(rc.relname, ''),
  array_to_string(ARRAY(SELECT a.attname FROM unnest(con.confkey) WITH ORDINALITY k(n, o) JOIN pg_attribute a ON a.attrelid = con.confrelid AND a.attnum = k.n ORDER BY k.o), ', ')
FROM pg_constraint con LEFT JOIN pg_class rc ON rc.oid = con.confrelid
WHERE con.conrelid = $1 ORDER BY con.contype = 'p' DESC, con.conname`, oid)
	if err != nil {
		return result, statementError(err)
	}
	constraints := []string{}
	for rows.Next() {
		var name, kind, definition, columns, refTable, refColumns string
		if err = rows.Scan(&name, &kind, &definition, &columns, &refTable, &refColumns); err != nil {
			_ = rows.Close()
			return result, statementError(err)
		}
		constraints = append(constraints, "CONSTRAINT "+s.quote(name)+" "+definition)
		if kind == "f" {
			result.ForeignKeys = append(result.ForeignKeys, ForeignKey{Name: name, Columns: columns, RefTable: refTable, RefColumns: refColumns, Definition: definition})
		}
	}
	_ = rows.Close()
	if result.Type == "view" || result.Type == "materialized view" {
		var definition string
		_ = s.conn.QueryRowContext(ctx, "SELECT pg_get_viewdef($1::oid, true)", oid).Scan(&definition)
		keyword := "VIEW"
		if result.Type == "materialized view" {
			keyword = "MATERIALIZED VIEW"
		}
		result.DDL = "CREATE " + keyword + " " + s.qualified(table) + " AS\n" + definition
		return result, nil
	}
	lines := []string{}
	for _, c := range result.Columns {
		line := "    " + s.quote(c.Name) + " " + c.Type
		if !c.Nullable {
			line += " NOT NULL"
		}
		if c.Default != "" {
			line += " DEFAULT " + c.Default
		}
		lines = append(lines, line)
	}
	for _, c := range constraints {
		lines = append(lines, "    "+c)
	}
	result.DDL = "CREATE TABLE " + s.qualified(table) + " (\n" + strings.Join(lines, ",\n") + "\n);"
	if len(standalone) > 0 {
		result.DDL += "\n\n" + strings.Join(standalone, "\n")
	}
	return result, nil
}

func (s *sqlSession) mysqlStructure(ctx context.Context, table string) (Structure, error) {
	result := Structure{Columns: []ColumnInfo{}, Indexes: []IndexInfo{}, ForeignKeys: []ForeignKey{}}
	var tableType string
	err := s.conn.QueryRowContext(ctx, "SELECT TABLE_TYPE FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?", table).Scan(&tableType)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrExternalInvalid
	}
	if err != nil {
		return result, statementError(err)
	}
	result.Type = "table"
	if strings.Contains(tableType, "VIEW") {
		result.Type = "view"
	}
	rows, err := s.conn.QueryContext(ctx, `SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE = 'YES', COALESCE(COLUMN_DEFAULT, ''),
  COLUMN_KEY = 'PRI', EXTRA LIKE '%auto_increment%', COALESCE(COLUMN_COMMENT, '')
FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? ORDER BY ORDINAL_POSITION`, table)
	if err != nil {
		return result, statementError(err)
	}
	for rows.Next() {
		var c ColumnInfo
		if err = rows.Scan(&c.Name, &c.Type, &c.Nullable, &c.Default, &c.Primary, &c.AutoIncrement, &c.Comment); err != nil {
			_ = rows.Close()
			return result, statementError(err)
		}
		result.Columns = append(result.Columns, c)
	}
	_ = rows.Close()
	rows, err = s.conn.QueryContext(ctx, `SELECT INDEX_NAME, MAX(NON_UNIQUE) = 0, INDEX_NAME = 'PRIMARY',
  GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX SEPARATOR ', ')
FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
GROUP BY INDEX_NAME ORDER BY INDEX_NAME = 'PRIMARY' DESC, INDEX_NAME`, table)
	if err != nil {
		return result, statementError(err)
	}
	for rows.Next() {
		var index IndexInfo
		if err = rows.Scan(&index.Name, &index.Unique, &index.Primary, &index.Columns); err != nil {
			_ = rows.Close()
			return result, statementError(err)
		}
		result.Indexes = append(result.Indexes, index)
	}
	_ = rows.Close()
	rows, err = s.conn.QueryContext(ctx, `SELECT CONSTRAINT_NAME, GROUP_CONCAT(COLUMN_NAME ORDER BY ORDINAL_POSITION SEPARATOR ', '),
  REFERENCED_TABLE_NAME, GROUP_CONCAT(REFERENCED_COLUMN_NAME ORDER BY ORDINAL_POSITION SEPARATOR ', ')
FROM information_schema.KEY_COLUMN_USAGE
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND REFERENCED_TABLE_NAME IS NOT NULL
GROUP BY CONSTRAINT_NAME, REFERENCED_TABLE_NAME ORDER BY CONSTRAINT_NAME`, table)
	if err != nil {
		return result, statementError(err)
	}
	for rows.Next() {
		var fk ForeignKey
		if err = rows.Scan(&fk.Name, &fk.Columns, &fk.RefTable, &fk.RefColumns); err != nil {
			_ = rows.Close()
			return result, statementError(err)
		}
		result.ForeignKeys = append(result.ForeignKeys, fk)
	}
	_ = rows.Close()
	var name string
	if result.Type == "view" {
		var definition, charset, collation string
		_ = s.conn.QueryRowContext(ctx, "SHOW CREATE VIEW "+s.quote(table)).Scan(&name, &definition, &charset, &collation)
		result.DDL = definition
	} else {
		_ = s.conn.QueryRowContext(ctx, "SHOW CREATE TABLE "+s.quote(table)).Scan(&name, &result.DDL)
	}
	return result, nil
}

func primaryKey(columns []ColumnInfo) []string {
	keys := []string{}
	for _, c := range columns {
		if c.Primary {
			keys = append(keys, c.Name)
		}
	}
	return keys
}

func (s *sqlSession) selectRows(ctx context.Context, r ClientRequest) (ResultSet, error) {
	structure, err := s.structure(ctx, r.Table)
	if err != nil {
		return ResultSet{}, err
	}
	known := map[string]bool{}
	for _, c := range structure.Columns {
		known[c.Name] = true
	}
	where, args, err := s.buildWhere(r.Filters, known)
	if err != nil {
		return ResultSet{}, err
	}
	limit := r.Limit
	if limit == 0 {
		limit = defaultPageRows
	}
	order := ""
	if r.OrderBy != "" {
		if !known[r.OrderBy] {
			return ResultSet{}, ErrExternalInvalid
		}
		order = " ORDER BY " + s.quote(r.OrderBy)
		if r.Desc {
			order += " DESC"
		}
	} else if keys := primaryKey(structure.Columns); len(keys) > 0 {
		quoted := make([]string, len(keys))
		for i, key := range keys {
			quoted[i] = s.quote(key)
		}
		order = " ORDER BY " + strings.Join(quoted, ", ")
	}
	started := time.Now()
	query := "SELECT * FROM " + s.qualified(r.Table) + where + order + " LIMIT " + strconv.Itoa(limit) + " OFFSET " + strconv.Itoa(r.Offset)
	rows, err := s.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return ResultSet{}, statementError(err)
	}
	result, err := scanResult(rows, limit)
	_ = rows.Close()
	if err != nil {
		return ResultSet{}, statementError(err)
	}
	result.DurationMS = time.Since(started).Milliseconds()
	result.PrimaryKey = primaryKey(structure.Columns)
	result.Total = -1
	countCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	var total int64
	if err = s.conn.QueryRowContext(countCtx, "SELECT COUNT(*) FROM "+s.qualified(r.Table)+where, args...).Scan(&total); err == nil {
		result.Total = total
	}
	return result, nil
}

func (s *sqlSession) buildWhere(filters []Filter, known map[string]bool) (string, []any, error) {
	parts := []string{}
	args := []any{}
	for _, f := range filters {
		if !known[f.Column] {
			return "", nil, ErrExternalInvalid
		}
		column := s.quote(f.Column)
		switch f.Op {
		case "IS NULL", "IS NOT NULL":
			parts = append(parts, column+" "+f.Op)
			continue
		case "LIKE", "NOT LIKE", "ILIKE":
			op := f.Op
			if s.engine == "postgres" {
				column = "CAST(" + column + " AS TEXT)"
			} else if op == "ILIKE" {
				op = "LIKE"
			}
			args = append(args, f.Value)
			parts = append(parts, column+" "+op+" "+s.placeholder(len(args)))
		default:
			args = append(args, f.Value)
			op := f.Op
			if op == "!=" {
				op = "<>"
			}
			parts = append(parts, column+" "+op+" "+s.placeholder(len(args)))
		}
	}
	if len(parts) == 0 {
		return "", args, nil
	}
	return " WHERE " + strings.Join(parts, " AND "), args, nil
}

func scanResult(rows *sql.Rows, limit int) (ResultSet, error) {
	result := ResultSet{Columns: []string{}, Types: []string{}, Rows: [][]any{}, PrimaryKey: []string{}}
	columns, err := rows.Columns()
	if err != nil {
		return result, err
	}
	result.Columns = columns
	if types, typeErr := rows.ColumnTypes(); typeErr == nil {
		for _, t := range types {
			result.Types = append(result.Types, strings.ToLower(t.DatabaseTypeName()))
		}
	}
	size := 0
	for rows.Next() {
		if len(result.Rows) >= limit {
			result.Truncated = true
			break
		}
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err = rows.Scan(pointers...); err != nil {
			return result, err
		}
		for i, value := range values {
			values[i] = jsonValue(value)
			if text, ok := values[i].(string); ok {
				size += len(text)
			} else {
				size += 16
			}
		}
		if size > maxClientPayload {
			result.Truncated = true
			break
		}
		result.Rows = append(result.Rows, values)
	}
	return result, rows.Err()
}

const maxSafeInteger = 1 << 53

func jsonValue(value any) any {
	switch v := value.(type) {
	case nil, bool, string:
		return v
	case []byte:
		if utf8.Valid(v) {
			return string(v)
		}
		return `\x` + hex.EncodeToString(v)
	case time.Time:
		return v.Format("2006-01-02 15:04:05.999999Z07:00")
	case int64:
		if v > maxSafeInteger || v < -maxSafeInteger {
			return strconv.FormatInt(v, 10)
		}
		return v
	case uint64:
		if v > maxSafeInteger {
			return strconv.FormatUint(v, 10)
		}
		return v
	case int, int8, int16, int32, uint, uint8, uint16, uint32, float32:
		return v
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return strconv.FormatFloat(v, 'g', -1, 64)
		}
		return v
	case map[string]any, []any:
		return v
	default:
		if encoded, err := json.Marshal(v); err == nil && len(encoded) > 0 && encoded[0] != '{' {
			var decoded any
			if json.Unmarshal(encoded, &decoded) == nil {
				return decoded
			}
		}
		return fmt.Sprint(v)
	}
}

var leadingNoise = regexp.MustCompile(`^(\s+|--[^\n]*\n?|#[^\n]*\n?|/\*[\s\S]*?\*/|\()+`)
var rowKeywords = map[string]bool{"SELECT": true, "WITH": true, "SHOW": true, "EXPLAIN": true, "DESCRIBE": true, "DESC": true, "VALUES": true, "TABLE": true, "PRAGMA": true}
var returningClause = regexp.MustCompile(`(?i)\bRETURNING\b`)

// returnsRows decides whether a statement is executed with Query (result set)
// or Exec (affected-row count).
func returnsRows(statement string) bool {
	rest := leadingNoise.ReplaceAllString(statement, "")
	end := strings.IndexFunc(rest, func(r rune) bool { return !unicode.IsLetter(r) })
	if end < 0 {
		end = len(rest)
	}
	keyword := strings.ToUpper(rest[:end])
	return rowKeywords[keyword] || returningClause.MatchString(statement)
}

func (s *sqlSession) query(ctx context.Context, text string) (QueryResult, error) {
	statements := SplitStatements(text, s.engine)
	if len(statements) == 0 || len(statements) > maxStatements {
		return QueryResult{}, ErrExternalInvalid
	}
	result := QueryResult{Results: []ResultSet{}}
	for _, statement := range statements {
		started := time.Now()
		var set ResultSet
		var err error
		if returnsRows(statement) {
			var rows *sql.Rows
			rows, err = s.conn.QueryContext(ctx, statement)
			if err == nil {
				set, err = scanResult(rows, maxQueryRows)
				_ = rows.Close()
			}
		} else {
			var res sql.Result
			res, err = s.conn.ExecContext(ctx, statement)
			set = ResultSet{Columns: []string{}, Types: []string{}, Rows: [][]any{}}
			if err == nil {
				set.Affected, _ = res.RowsAffected()
			}
		}
		set.PrimaryKey = []string{}
		set.Total = -1
		set.Statement = truncateMessage(statement)
		set.DurationMS = time.Since(started).Milliseconds()
		if err != nil {
			set.Error = statementError(err).Error()
			result.Results = append(result.Results, set)
			break
		}
		result.Results = append(result.Results, set)
	}
	return result, nil
}

func (s *sqlSession) argument(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		return v
	case bool:
		if s.engine == "mysql" {
			if v {
				return "1"
			}
			return "0"
		}
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		encoded, _ := json.Marshal(v)
		return string(encoded)
	}
}

func (s *sqlSession) keyCondition(key map[string]any, keys []string, start int) (string, []any, error) {
	parts := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys))
	for _, column := range keys {
		value, ok := key[column]
		if !ok || value == nil {
			return "", nil, ErrExternalInvalid
		}
		args = append(args, s.argument(value))
		parts = append(parts, s.quote(column)+" = "+s.placeholder(start+len(args)))
	}
	return strings.Join(parts, " AND "), args, nil
}

func (s *sqlSession) modifyRows(ctx context.Context, r ClientRequest) (RowsResult, error) {
	structure, err := s.structure(ctx, r.Table)
	if err != nil {
		return RowsResult{}, err
	}
	if structure.Type != "table" {
		return RowsResult{}, ErrUnsupported
	}
	known := map[string]bool{}
	for _, c := range structure.Columns {
		known[c.Name] = true
	}
	for column := range r.Values {
		if !known[column] {
			return RowsResult{}, ErrExternalInvalid
		}
	}
	// Deterministic column order keeps statements reproducible.
	columns := []string{}
	for _, c := range structure.Columns {
		if _, ok := r.Values[c.Name]; ok {
			columns = append(columns, c.Name)
		}
	}
	table := s.qualified(r.Table)
	if r.Action == "insert" {
		if len(columns) == 0 {
			statement := "INSERT INTO " + table + " DEFAULT VALUES"
			if s.engine == "mysql" {
				statement = "INSERT INTO " + table + " () VALUES ()"
			}
			res, execErr := s.conn.ExecContext(ctx, statement)
			if execErr != nil {
				return RowsResult{}, statementError(execErr)
			}
			affected, _ := res.RowsAffected()
			return RowsResult{Affected: affected}, nil
		}
		quoted := make([]string, len(columns))
		holders := make([]string, len(columns))
		args := make([]any, len(columns))
		for i, column := range columns {
			quoted[i] = s.quote(column)
			holders[i] = s.placeholder(i + 1)
			args[i] = s.argument(r.Values[column])
		}
		res, execErr := s.conn.ExecContext(ctx, "INSERT INTO "+table+" ("+strings.Join(quoted, ", ")+") VALUES ("+strings.Join(holders, ", ")+")", args...)
		if execErr != nil {
			return RowsResult{}, statementError(execErr)
		}
		affected, _ := res.RowsAffected()
		return RowsResult{Affected: affected}, nil
	}
	keys := primaryKey(structure.Columns)
	if len(keys) == 0 {
		return RowsResult{}, ErrNoPrimaryKey
	}
	if len(r.Keys) == 0 || (r.Action == "update" && (len(r.Keys) != 1 || len(columns) == 0)) {
		return RowsResult{}, ErrExternalInvalid
	}
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return RowsResult{}, statementError(err)
	}
	defer func() { _ = tx.Rollback() }()
	var affected int64
	for _, key := range r.Keys {
		var statement string
		var args []any
		if r.Action == "update" {
			sets := make([]string, len(columns))
			for i, column := range columns {
				args = append(args, s.argument(r.Values[column]))
				sets[i] = s.quote(column) + " = " + s.placeholder(len(args))
			}
			condition, keyArgs, keyErr := s.keyCondition(key, keys, len(args))
			if keyErr != nil {
				return RowsResult{}, keyErr
			}
			args = append(args, keyArgs...)
			statement = "UPDATE " + table + " SET " + strings.Join(sets, ", ") + " WHERE " + condition
		} else {
			condition, keyArgs, keyErr := s.keyCondition(key, keys, 0)
			if keyErr != nil {
				return RowsResult{}, keyErr
			}
			args = keyArgs
			statement = "DELETE FROM " + table + " WHERE " + condition
		}
		res, execErr := tx.ExecContext(ctx, statement, args...)
		if execErr != nil {
			return RowsResult{}, statementError(execErr)
		}
		count, _ := res.RowsAffected()
		affected += count
	}
	if err = tx.Commit(); err != nil {
		return RowsResult{}, statementError(err)
	}
	return RowsResult{Affected: affected}, nil
}
