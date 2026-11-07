package database

// External browsing is separate from the panel's own SQLite repositories.
// Connections are request scoped; credentials are never persisted or logged.
import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var ErrExternalInvalid = errors.New("invalid database browser request")
var ErrExternalTooLarge = errors.New("database result exceeds response limit")
var externalName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
var externalHost = regexp.MustCompile(`^[a-zA-Z0-9._:-]{1,253}$`)

type ExternalRequest struct {
	Engine     string `json:"engine"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Database   string `json:"database"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	TLS        bool   `json:"tls"`
	AuthSource string `json:"auth_source"`
	Table      string `json:"table"`
	Offset     int    `json:"offset"`
}
type ExternalResult struct {
	Tables  []string         `json:"tables"`
	Rows    []map[string]any `json:"rows"`
	Columns []string         `json:"columns"`
	HasMore bool             `json:"has_more"`
}

func ValidateExternal(r ExternalRequest) error {
	if r.Engine != "postgres" && r.Engine != "mysql" && r.Engine != "mongodb" {
		return ErrExternalInvalid
	}
	if !externalHost.MatchString(r.Host) || r.Port < 1 || r.Port > 65535 || !externalName.MatchString(r.Database) || len(r.Username) > 256 || len(r.Password) > 4096 || strings.ContainsAny(r.Username+r.Password, "\x00") || (r.Table != "" && !externalName.MatchString(r.Table)) || r.Offset < 0 || r.Offset > 100000 || (r.AuthSource != "" && !externalName.MatchString(r.AuthSource)) {
		return ErrExternalInvalid
	}
	return nil
}

func BrowseExternal(ctx context.Context, r ExternalRequest) (ExternalResult, error) {
	result := ExternalResult{Tables: []string{}, Rows: []map[string]any{}, Columns: []string{}}
	if err := ValidateExternal(r); err != nil {
		return result, err
	}
	var err error
	if r.Engine == "mongodb" {
		result, err = browseMongo(ctx, r)
	} else {
		result, err = browseSQL(ctx, r)
	}
	if err != nil {
		return result, err
	}
	sort.Strings(result.Tables)
	columns := map[string]bool{}
	for _, row := range result.Rows {
		for key, v := range row {
			columns[key] = true
			if b, ok := v.([]byte); ok {
				row[key] = string(b)
			}
		}
	}
	for key := range columns {
		result.Columns = append(result.Columns, key)
	}
	sort.Strings(result.Columns)
	encoded, err := json.Marshal(result)
	if err != nil {
		return result, err
	}
	if len(encoded) > 4<<20 {
		return ExternalResult{}, ErrExternalTooLarge
	}
	return result, nil
}

// sqlDialector builds a request-scoped PostgreSQL or MySQL dialector. The
// statement timeout bounds server-side work even if the client disconnects.
func sqlDialector(r ExternalRequest, timeout time.Duration) gorm.Dialector {
	addr := net.JoinHostPort(r.Host, strconv.Itoa(r.Port))
	if r.Engine == "postgres" {
		u := url.URL{Scheme: "postgres", Host: addr, Path: "/" + r.Database, User: url.UserPassword(r.Username, r.Password)}
		q := u.Query()
		q.Set("sslmode", "disable")
		if r.TLS {
			q.Set("sslmode", "verify-full")
		}
		q.Set("connect_timeout", "5")
		q.Set("statement_timeout", strconv.FormatInt(timeout.Milliseconds(), 10))
		q.Set("application_name", "mini-ubuntu-server")
		u.RawQuery = q.Encode()
		return postgres.New(postgres.Config{DSN: u.String(), PreferSimpleProtocol: true})
	}
	config := mysqldriver.NewConfig()
	config.User = r.Username
	config.Passwd = r.Password
	config.Net = "tcp"
	config.Addr = addr
	config.DBName = r.Database
	config.Timeout = 5 * time.Second
	config.ReadTimeout = timeout
	config.WriteTimeout = timeout
	if r.TLS {
		config.TLSConfig = "true"
	}
	return mysql.New(mysql.Config{DSN: config.FormatDSN(), SkipInitializeWithVersion: true})
}

func browseSQL(ctx context.Context, r ExternalRequest) (ExternalResult, error) {
	result := ExternalResult{Tables: []string{}, Rows: []map[string]any{}, Columns: []string{}}
	dialector := sqlDialector(r, 8*time.Second)
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
	if err != nil {
		return result, err
	}
	pool, err := db.DB()
	if err != nil {
		return result, err
	}
	defer func() { _ = pool.Close() }()
	pool.SetMaxOpenConns(1)
	pool.SetMaxIdleConns(0)
	db = db.WithContext(ctx)
	tables, err := db.Migrator().GetTables()
	if err != nil {
		return result, err
	}
	found := false
	for _, table := range tables {
		if externalName.MatchString(table) {
			result.Tables = append(result.Tables, table)
			if table == r.Table {
				found = true
			}
		}
	}
	if r.Table == "" {
		return result, nil
	}
	if !found {
		return result, ErrExternalInvalid
	}
	// Identifiers are validated, selected from server metadata, and quoted by GORM.
	query := db.Table(r.Table).Clauses(clause.From{Tables: []clause.Table{{Name: r.Table}}})
	columns, err := db.Migrator().ColumnTypes(r.Table)
	if err != nil {
		return result, err
	}
	for _, col := range columns {
		if primary, ok := col.PrimaryKey(); ok && primary {
			query = query.Order(clause.OrderByColumn{Column: clause.Column{Name: col.Name()}})
		}
	}
	err = query.Offset(r.Offset).Limit(101).Find(&result.Rows).Error
	if len(result.Rows) > 100 {
		result.HasMore = true
		result.Rows = result.Rows[:100]
	}
	return result, err
}

// connectMongo opens a request-scoped client and verifies it with a ping.
func connectMongo(ctx context.Context, r ExternalRequest) (*mongo.Client, error) {
	u := url.URL{Scheme: "mongodb", Host: net.JoinHostPort(r.Host, strconv.Itoa(r.Port)), Path: "/" + r.Database}
	if r.Username != "" {
		u.User = url.UserPassword(r.Username, r.Password)
	}
	q := u.Query()
	if r.AuthSource != "" {
		q.Set("authSource", r.AuthSource)
	}
	if r.TLS {
		q.Set("tls", "true")
	}
	u.RawQuery = q.Encode()
	client, err := mongo.Connect(options.Client().ApplyURI(u.String()).SetServerSelectionTimeout(5 * time.Second).SetConnectTimeout(5 * time.Second).SetMaxPoolSize(1))
	if err != nil {
		return nil, err
	}
	if err = client.Ping(ctx, readpref.Primary()); err != nil {
		disconnectMongo(client)
		return nil, err
	}
	return client, nil
}

func disconnectMongo(client *mongo.Client) {
	closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = client.Disconnect(closeCtx)
}

func browseMongo(ctx context.Context, r ExternalRequest) (ExternalResult, error) {
	result := ExternalResult{Tables: []string{}, Rows: []map[string]any{}, Columns: []string{}}
	client, err := connectMongo(ctx, r)
	if err != nil {
		return result, err
	}
	defer disconnectMongo(client)
	db := client.Database(r.Database)
	tables, err := db.ListCollectionNames(ctx, bson.D{})
	if err != nil {
		return result, err
	}
	found := false
	for _, table := range tables {
		if externalName.MatchString(table) {
			result.Tables = append(result.Tables, table)
			if table == r.Table {
				found = true
			}
		}
	}
	if r.Table == "" {
		return result, nil
	}
	if !found {
		return result, ErrExternalInvalid
	}
	cursor, err := db.Collection(r.Table).Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetSkip(int64(r.Offset)).SetLimit(101).SetBatchSize(10))
	if err != nil {
		return result, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	size := 0
	for cursor.Next(ctx) {
		size += len(cursor.Current)
		if size > 4<<20 {
			return result, ErrExternalTooLarge
		}
		var doc bson.M
		if err = cursor.Decode(&doc); err != nil {
			return result, err
		}
		encoded, e := bson.MarshalExtJSON(doc, false, false)
		if e != nil {
			return result, e
		}
		var row map[string]any
		if err = json.Unmarshal(encoded, &row); err != nil {
			return result, err
		}
		result.Rows = append(result.Rows, row)
	}
	if len(result.Rows) > 100 {
		result.HasMore = true
		result.Rows = result.Rows[:100]
	}
	return result, cursor.Err()
}
