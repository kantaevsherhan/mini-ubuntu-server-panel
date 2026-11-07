package database

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func mongoError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return StatementError{Message: "query timed out"}
	}
	var commandErr mongo.CommandError
	if errors.As(err, &commandErr) {
		return StatementError{Message: truncateMessage(commandErr.Message)}
	}
	var writeErr mongo.WriteException
	if errors.As(err, &writeErr) && len(writeErr.WriteErrors) > 0 {
		return StatementError{Message: truncateMessage(writeErr.WriteErrors[0].Message)}
	}
	return StatementError{Message: "operation failed"}
}

func parseDocument(text string) (bson.D, error) {
	var doc bson.D
	if strings.TrimSpace(text) == "" {
		return bson.D{}, nil
	}
	if err := bson.UnmarshalExtJSON([]byte(text), false, &doc); err != nil {
		return nil, StatementError{Message: "invalid JSON document: " + truncateMessage(err.Error())}
	}
	return doc, nil
}

func relaxedValue(raw bson.Raw) (map[string]any, string, error) {
	encoded, err := bson.MarshalExtJSON(raw, false, false)
	if err != nil {
		return nil, "", err
	}
	var row map[string]any
	if err = json.Unmarshal(encoded, &row); err != nil {
		return nil, "", err
	}
	pretty, err := bson.MarshalExtJSONIndent(raw, false, false, "", "  ")
	if err != nil {
		return nil, "", err
	}
	return row, string(pretty), nil
}

func runMongo(ctx context.Context, op string, r ClientRequest) (any, error) {
	client, err := connectMongo(ctx, r.external())
	if err != nil {
		return nil, err
	}
	defer disconnectMongo(client)
	db := client.Database(r.Database)
	collectionExists := func() error {
		names, listErr := db.ListCollectionNames(ctx, bson.D{{Key: "name", Value: r.Table}})
		if listErr != nil {
			return mongoError(listErr)
		}
		if r.Table == "" || len(names) == 0 {
			return ErrExternalInvalid
		}
		return nil
	}
	switch op {
	case "overview":
		return mongoOverview(ctx, client, db)
	case "structure":
		if err = collectionExists(); err != nil {
			return nil, err
		}
		return mongoStructure(ctx, db.Collection(r.Table))
	case "select":
		if err = collectionExists(); err != nil {
			return nil, err
		}
		return mongoSelect(ctx, db.Collection(r.Table), r)
	case "query":
		return mongoCommand(ctx, db, r.SQL)
	default:
		if err = collectionExists(); err != nil {
			return nil, err
		}
		return mongoModify(ctx, db.Collection(r.Table), r)
	}
}

func mongoOverview(ctx context.Context, client *mongo.Client, db *mongo.Database) (Overview, error) {
	result := Overview{Databases: []string{}, Schemas: []string{}, Tables: []TableInfo{}}
	var info bson.M
	if client.Database("admin").RunCommand(ctx, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&info) == nil {
		if version, ok := info["version"].(string); ok {
			result.Version = "MongoDB " + version
		}
	}
	if names, err := client.ListDatabaseNames(ctx, bson.D{}); err == nil {
		sort.Strings(names)
		result.Databases = names
	}
	specs, err := db.ListCollectionSpecifications(ctx, bson.D{})
	if err != nil {
		return result, mongoError(err)
	}
	if len(specs) > 1000 {
		specs = specs[:1000]
	}
	for _, spec := range specs {
		table := TableInfo{Name: spec.Name, Type: "collection"}
		if spec.Type == "view" {
			table.Type = "view"
		}
		countCtx, cancel := context.WithTimeout(ctx, time.Second)
		if count, countErr := db.Collection(spec.Name).EstimatedDocumentCount(countCtx); countErr == nil {
			table.Rows = count
		}
		cancel()
		result.Tables = append(result.Tables, table)
	}
	sort.Slice(result.Tables, func(i, j int) bool { return result.Tables[i].Name < result.Tables[j].Name })
	return result, nil
}

func mongoStructure(ctx context.Context, collection *mongo.Collection) (Structure, error) {
	result := Structure{Type: "collection", Columns: []ColumnInfo{}, Indexes: []IndexInfo{}, ForeignKeys: []ForeignKey{}}
	cursor, err := collection.Find(ctx, bson.D{}, options.Find().SetLimit(200))
	if err != nil {
		return result, mongoError(err)
	}
	defer func() { _ = cursor.Close(ctx) }()
	types := map[string]map[string]bool{}
	order := []string{}
	for cursor.Next(ctx) {
		elements, elemErr := cursor.Current.Elements()
		if elemErr != nil {
			return result, mongoError(elemErr)
		}
		for _, element := range elements {
			key := element.Key()
			if types[key] == nil {
				types[key] = map[string]bool{}
				order = append(order, key)
			}
			types[key][element.Value().Type.String()] = true
		}
	}
	for _, key := range order {
		names := []string{}
		for name := range types[key] {
			names = append(names, name)
		}
		sort.Strings(names)
		result.Columns = append(result.Columns, ColumnInfo{Name: key, Type: strings.Join(names, " | "), Nullable: key != "_id", Primary: key == "_id"})
	}
	specs, err := collection.Indexes().ListSpecifications(ctx)
	if err == nil {
		for _, spec := range specs {
			keys := []string{}
			var doc bson.D
			if bson.Unmarshal(spec.KeysDocument, &doc) == nil {
				for _, element := range doc {
					keys = append(keys, element.Key)
				}
			}
			unique := spec.Unique != nil && *spec.Unique
			definition, _ := bson.MarshalExtJSON(spec.KeysDocument, false, false)
			result.Indexes = append(result.Indexes, IndexInfo{Name: spec.Name, Columns: strings.Join(keys, ", "), Unique: unique || spec.Name == "_id_", Primary: spec.Name == "_id_", Definition: string(definition)})
		}
	}
	return result, nil
}

func mongoSelect(ctx context.Context, collection *mongo.Collection, r ClientRequest) (ResultSet, error) {
	result := ResultSet{Columns: []string{}, Types: []string{}, Rows: [][]any{}, Documents: []string{}, PrimaryKey: []string{"_id"}, Total: -1}
	filter, err := parseDocument(r.MongoFilter)
	if err != nil {
		return result, err
	}
	limit := r.Limit
	if limit == 0 {
		limit = defaultPageRows
	}
	sortKey := "_id"
	if r.OrderBy != "" {
		sortKey = r.OrderBy
	}
	direction := 1
	if r.Desc {
		direction = -1
	}
	started := time.Now()
	cursor, err := collection.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: sortKey, Value: direction}}).SetSkip(int64(r.Offset)).SetLimit(int64(limit)).SetBatchSize(50))
	if err != nil {
		return result, mongoError(err)
	}
	defer func() { _ = cursor.Close(ctx) }()
	rows := []map[string]any{}
	columns := map[string]bool{}
	order := []string{}
	size := 0
	for cursor.Next(ctx) {
		size += len(cursor.Current)
		if size > maxClientPayload/2 {
			result.Truncated = true
			break
		}
		row, pretty, decodeErr := relaxedValue(cursor.Current)
		if decodeErr != nil {
			return result, mongoError(decodeErr)
		}
		elements, _ := cursor.Current.Elements()
		for _, element := range elements {
			if !columns[element.Key()] {
				columns[element.Key()] = true
				order = append(order, element.Key())
			}
		}
		rows = append(rows, row)
		result.Documents = append(result.Documents, pretty)
	}
	if err = cursor.Err(); err != nil {
		return result, mongoError(err)
	}
	result.Columns = order
	for _, row := range rows {
		values := make([]any, len(order))
		for i, key := range order {
			values[i] = row[key]
		}
		result.Rows = append(result.Rows, values)
	}
	result.DurationMS = time.Since(started).Milliseconds()
	countCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	if total, countErr := collection.CountDocuments(countCtx, filter); countErr == nil {
		result.Total = total
	}
	return result, nil
}

// mongoCommand runs a database command document such as
// {"find": "users", "filter": {"active": true}} or {"aggregate": ...}.
func mongoCommand(ctx context.Context, db *mongo.Database, text string) (QueryResult, error) {
	command, err := parseDocument(text)
	if err != nil {
		return QueryResult{}, err
	}
	if len(command) == 0 {
		return QueryResult{}, ErrExternalInvalid
	}
	started := time.Now()
	set := ResultSet{Statement: truncateMessage(strings.TrimSpace(text)), Columns: []string{}, Types: []string{}, Rows: [][]any{}, Documents: []string{}, PrimaryKey: []string{}, Total: -1}
	raw, err := db.RunCommand(ctx, command).Raw()
	set.DurationMS = time.Since(started).Milliseconds()
	if err != nil {
		set.Error = mongoError(err).Error()
		return QueryResult{Results: []ResultSet{set}}, nil
	}
	// Commands like find/aggregate return documents in cursor.firstBatch.
	if batch, lookupErr := raw.LookupErr("cursor", "firstBatch"); lookupErr == nil {
		if docs, ok := batch.ArrayOK(); ok {
			values, _ := docs.Values()
			columns := map[string]bool{}
			parsed := []map[string]any{}
			for _, value := range values {
				doc, isDoc := value.DocumentOK()
				if !isDoc {
					continue
				}
				row, pretty, decodeErr := relaxedValue(doc)
				if decodeErr != nil {
					continue
				}
				elements, _ := doc.Elements()
				for _, element := range elements {
					if !columns[element.Key()] {
						columns[element.Key()] = true
						set.Columns = append(set.Columns, element.Key())
					}
				}
				parsed = append(parsed, row)
				set.Documents = append(set.Documents, pretty)
			}
			for _, row := range parsed {
				values := make([]any, len(set.Columns))
				for i, key := range set.Columns {
					values[i] = row[key]
				}
				set.Rows = append(set.Rows, values)
			}
			return QueryResult{Results: []ResultSet{set}}, nil
		}
	}
	_, pretty, err := relaxedValue(raw)
	if err != nil {
		return QueryResult{}, mongoError(err)
	}
	set.Columns = []string{"result"}
	set.Rows = [][]any{{pretty}}
	set.Documents = []string{pretty}
	return QueryResult{Results: []ResultSet{set}}, nil
}

func mongoKey(key map[string]any) (bson.D, error) {
	id, ok := key["_id"]
	if !ok {
		return nil, ErrExternalInvalid
	}
	encoded, err := json.Marshal(map[string]any{"_id": id})
	if err != nil {
		return nil, ErrExternalInvalid
	}
	var filter bson.D
	if err = bson.UnmarshalExtJSON(encoded, false, &filter); err != nil {
		return nil, ErrExternalInvalid
	}
	return filter, nil
}

func mongoModify(ctx context.Context, collection *mongo.Collection, r ClientRequest) (RowsResult, error) {
	switch r.Action {
	case "insert":
		doc, err := parseDocument(r.Document)
		if err != nil {
			return RowsResult{}, err
		}
		if _, err = collection.InsertOne(ctx, doc); err != nil {
			return RowsResult{}, mongoError(err)
		}
		return RowsResult{Affected: 1}, nil
	case "update":
		if len(r.Keys) != 1 {
			return RowsResult{}, ErrExternalInvalid
		}
		filter, err := mongoKey(r.Keys[0])
		if err != nil {
			return RowsResult{}, err
		}
		doc, err := parseDocument(r.Document)
		if err != nil {
			return RowsResult{}, err
		}
		replacement := bson.D{}
		for _, element := range doc {
			if element.Key != "_id" {
				replacement = append(replacement, element)
			}
		}
		res, err := collection.ReplaceOne(ctx, filter, replacement)
		if err != nil {
			return RowsResult{}, mongoError(err)
		}
		return RowsResult{Affected: res.ModifiedCount}, nil
	default:
		if len(r.Keys) == 0 {
			return RowsResult{}, ErrExternalInvalid
		}
		var affected int64
		for _, key := range r.Keys {
			filter, err := mongoKey(key)
			if err != nil {
				return RowsResult{}, err
			}
			res, err := collection.DeleteOne(ctx, filter)
			if err != nil {
				return RowsResult{}, mongoError(err)
			}
			affected += res.DeletedCount
		}
		return RowsResult{Affected: affected}, nil
	}
}
