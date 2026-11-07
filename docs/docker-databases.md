# Docker resources and database client

Administrators configure container CPU/RAM limits; operators can inspect statistics. Database browsing is restricted to administrators. The page keeps credentials only in memory and clears them on disconnect/navigation. The backend opens a request-scoped connection and never stores credentials or returned records. Audit entries contain engine and outcome only.

Docker: `/docker` lists CPU (100% = one core), RAM excluding inactive file cache, effective RAM ceiling, cumulative network and disk bytes, and PIDs. Polling runs every 30 seconds while the page and container tab are visible, can be paused, and does not overlap. Failed samples show unavailable rather than zero usage. Container creation and the row menu expose CPU cores and RAM in MiB; zero means unlimited CPU; for RAM zero means unlimited at creation and leaves the current limit unchanged during update (Docker cannot remove a running memory limit). RAM reduction can terminate container processes and is confirmed before applying. Docker daemon errors leave the form available for retry. Docker-managed limits can be overwritten by a later Compose redeployment.

Database client: `/databases` is an Adminer-style client for PostgreSQL, MySQL/MariaDB and MongoDB, restricted to administrators.

- Connect: running database containers are listed under "Databases in Docker" (detected by image: postgres/postgis/timescale, mysql/mariadb/percona, mongo). One click connects using the container IP and the official image variables (`POSTGRES_USER`/`POSTGRES_PASSWORD`/`POSTGRES_DB`, `MYSQL_*`/`MARIADB_*`, `MONGO_INITDB_*`). The password is resolved on the server for every request and never sent to the browser. Manual connections take explicit host, port, database, credentials and optional certificate-verified TLS; the address is resolved by the panel server.
- Navigate: switch database and PostgreSQL schema, filter the table list, see tables/views/collections with estimated rows, size and comments.
- Data: filter (`=`, `!=`, `<`, `>`, `<=`, `>=`, `LIKE`, `NOT LIKE`, `ILIKE`, `IS [NOT] NULL`; MongoDB uses a JSON filter), sort by any column, page with exact totals (count is skipped after 4 seconds), export the page as CSV.
- Edit: insert, edit, clone and delete rows (bulk delete in one transaction). Updates and deletes address rows by primary key; tables without one are read-only in the grid and can be changed with SQL. Only changed columns are sent; NULL and server defaults are explicit choices. MongoDB documents are edited as extended JSON and addressed by `_id`.
- Structure: columns with type, NULL, default, PK/auto-increment and comments, indexes, foreign keys (click to open the referenced table) and DDL.
- SQL: Monaco editor with PostgreSQL/MySQL highlighting and table/column completion. Scripts are split on top-level `;` (strings, comments and `$$` bodies are respected) and run sequentially on one connection, so `BEGIN`/`COMMIT` work; execution stops at the first error and every statement shows rows or affected count and duration. Ctrl+Enter runs the selection or the whole script. MongoDB accepts a JSON database command (`{"find": ...}`, `{"aggregate": ...}`). Query history is kept in page memory only.

Statements run with the database user's permissions and apply immediately: use a read-only account when only browsing. Credentials, SQL text and returned data are never stored or logged; audit records `database.query` (statement keywords only) and `database.rows` (table, action, row count). Database error messages (syntax errors, constraint violations) are shown to the administrator verbatim because they describe their own statements; connection errors stay generic.

API:

- `GET /api/v1/docker/stats`: admin/operator; ID → sample, limits and per-container failure flag. Four workers, 12-second total deadline.
- `PUT /api/v1/docker/containers/:id/limits`: admin; `{cpus, memory_mb}`. Validated, audited, immediate Docker update. Existing NanoCPU-based limits can be adjusted but resetting those to unlimited requires Docker CLI/Compose.
- `POST /api/v1/databases/browse`: admin; legacy read-only browser kept for API compatibility. `{engine, host, port, database, username, password, tls, auth_source, table, offset}` → `{tables, columns, rows, has_more}`.
- `GET /api/v1/databases/containers`: admin; running database containers `{id, name, image, engine, host, port, database, username, auth_source, has_password}` (no passwords). Empty list when Docker access is disabled.
- `POST /api/v1/databases/{overview|structure|select|query|rows}`: admin; body = connection `{engine, host, port, database, username, password, tls, auth_source, schema}` or `{container}` (address and credentials filled on the server; explicit fields win) plus:
  - `overview` → `{version, databases, schemas, schema, tables: [{name, type, rows, size, comment}]}`;
  - `structure` `{table}` → `{type, columns, indexes, foreign_keys, ddl}`;
  - `select` `{table, offset, limit ≤ 500, order_by, desc, filters: [{column, op, value}], mongo_filter}` → `{columns, types, rows (arrays), documents (MongoDB), primary_key, total (-1 unknown), duration_ms, truncated}`;
  - `query` `{sql}` (≤ 1 MiB, ≤ 50 statements, ≤ 1000 rows each) → `{results: [{statement, columns, rows, affected, duration_ms, truncated, error}]}`;
  - `rows` `{table, action: insert|update|delete, keys: [{pk: value}], values: {column: value|null}, document}` → `{affected}`.

  Four concurrent requests globally, 15-second deadline (32 seconds for `query`), 8 MiB response limit, no-store responses. Stable errors: `database_request_invalid` (422), `database_statement_failed` (400, with the database `message`), `database_no_primary_key` (409), `database_unsupported` (422), `database_container_not_found` (404), `database_busy` (429), `database_result_too_large` (413), `database_connection_failed` (502).

No persistent entities or migrations are added. The browser keeps recent manual connections without passwords in `localStorage`, and only Docker-container sessions (which carry no password) survive a page reload via `sessionStorage`. UI uses the existing PrimeVue components, themes, Russian and English dictionaries.
