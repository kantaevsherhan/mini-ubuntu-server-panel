import api from './api'

export type Engine = 'postgres' | 'mysql' | 'mongodb'

/** Connection parameters. The password lives only in page memory. */
export interface Connection {
  engine: Engine
  host: string
  port: number
  database: string
  username: string
  password: string
  auth_source: string
  tls: boolean
  /** Docker container ID: the server fills address and credentials itself. */
  container: string
  schema: string
}

export interface ContainerTarget {
  id: string
  name: string
  image: string
  engine: Engine
  host: string
  port: number
  database: string
  username: string
  auth_source: string
  has_password: boolean
}

export interface TableInfo {
  name: string
  type: string
  rows: number
  size: number
  comment: string
}

export interface Overview {
  version: string
  databases: string[]
  schemas: string[]
  schema: string
  tables: TableInfo[]
}

export interface ColumnInfo {
  name: string
  type: string
  nullable: boolean
  default: string
  primary: boolean
  auto_increment: boolean
  comment: string
}

export interface Structure {
  type: string
  columns: ColumnInfo[]
  indexes: Array<{
    name: string
    columns: string
    unique: boolean
    primary: boolean
    definition: string
  }>
  foreign_keys: Array<{
    name: string
    columns: string
    ref_table: string
    ref_columns: string
    definition: string
  }>
  ddl: string
}

export type Cell = string | number | boolean | null | Record<string, unknown> | unknown[]

export interface ResultSet {
  statement?: string
  columns: string[]
  types: string[]
  rows: Cell[][]
  documents?: string[]
  primary_key: string[]
  total: number
  affected: number
  duration_ms: number
  truncated: boolean
  error?: string
}

export interface Filter {
  column: string
  op: string
  value: string
}

export const filterOperators = [
  '=',
  '!=',
  '<',
  '>',
  '<=',
  '>=',
  'LIKE',
  'NOT LIKE',
  'ILIKE',
  'IS NULL',
  'IS NOT NULL',
]

export const defaultPorts: Record<Engine, number> = { postgres: 5432, mysql: 3306, mongodb: 27017 }

export const engineLabels: Record<Engine, string> = {
  postgres: 'PostgreSQL',
  mysql: 'MySQL / MariaDB',
  mongodb: 'MongoDB',
}

/** Requests are silent: the page renders database errors inline, Adminer-style. */
async function call<T>(op: string, connection: Connection, extra: object = {}, timeout = 20000) {
  const { data } = await api.post<T>(
    `/databases/${op}`,
    { ...connection, ...extra },
    { silent: true, timeout },
  )
  return data
}

export const databaseClient = {
  containers: async () =>
    (await api.get<ContainerTarget[]>('/databases/containers', { silent: true })).data,
  overview: (c: Connection) => call<Overview>('overview', c),
  structure: (c: Connection, table: string) => call<Structure>('structure', c, { table }),
  select: (
    c: Connection,
    params: {
      table: string
      offset: number
      limit: number
      order_by?: string
      desc?: boolean
      filters?: Filter[]
      mongo_filter?: string
    },
  ) => call<ResultSet>('select', c, params),
  query: (c: Connection, sql: string) => call<{ results: ResultSet[] }>('query', c, { sql }, 40000),
  rows: (
    c: Connection,
    params: {
      table: string
      action: 'insert' | 'update' | 'delete'
      keys?: Record<string, Cell>[]
      values?: Record<string, Cell>
      document?: string
    },
  ) => call<{ affected: number }>('rows', c, params),
}

/** Error text for a failed client request: database message or a localized code. */
export function errorText(error: unknown, labels: Record<string, string>, fallback: string) {
  const response = (error as { response?: { data?: { error?: string; message?: string } } })
    ?.response
  const message = response?.data?.message
  if (typeof message === 'string' && message) return message
  const code = response?.data?.error
  return (code && labels[code]) || fallback
}

export function cellText(value: Cell | undefined): string {
  if (value === null || value === undefined) return 'NULL'
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

/** Editable text for a cell: objects become JSON, NULL becomes empty. */
export function editText(value: Cell | undefined): string {
  if (value === null || value === undefined) return ''
  if (typeof value === 'object') return JSON.stringify(value, null, 2)
  return String(value)
}

export function formatBytes(bytes = 0) {
  if (!bytes) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / 1024 ** index).toFixed(index ? 1 : 0)} ${units[index]}`
}

export function toCSV(columns: string[], rows: Cell[][]) {
  const escape = (value: Cell | undefined) => {
    if (value === null || value === undefined) return ''
    const text = typeof value === 'object' ? JSON.stringify(value) : String(value)
    return /[",\n\r;]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text
  }
  return [columns, ...rows].map((row) => row.map((v) => escape(v as Cell)).join(',')).join('\r\n')
}

export function downloadText(filename: string, text: string, type = 'text/csv') {
  const url = URL.createObjectURL(new Blob(['﻿' + text], { type: `${type};charset=utf-8` }))
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

/** Recent manual connections, without passwords. */
const recentKey = 'database-recent-connections'
export type RecentConnection = Omit<Connection, 'password'>

export function loadRecent(): RecentConnection[] {
  try {
    const parsed = JSON.parse(localStorage.getItem(recentKey) || '[]')
    return Array.isArray(parsed) ? parsed.slice(0, 8) : []
  } catch {
    return []
  }
}

export function saveRecent(connection: Connection) {
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  const { password, ...safe } = connection
  const key = (c: RecentConnection) =>
    `${c.container}|${c.engine}|${c.host}|${c.port}|${c.database}|${c.username}`
  const list = [safe, ...loadRecent().filter((item) => key(item) !== key(safe))].slice(0, 8)
  try {
    localStorage.setItem(recentKey, JSON.stringify(list))
  } catch {
    /* storage unavailable: recent list is a convenience only */
  }
}

export function forgetRecent(index: number) {
  const list = loadRecent()
  list.splice(index, 1)
  try {
    localStorage.setItem(recentKey, JSON.stringify(list))
  } catch {
    /* ignore */
  }
}

// Query history stays in page memory only: SQL may contain secrets.
let history: string[] = []
export function loadHistory(): string[] {
  return history
}

export function pushHistory(sql: string) {
  const text = sql.trim()
  if (!text || text.length > 20000) return history
  history = [text, ...history.filter((item) => item !== text)].slice(0, 30)
  return history
}
