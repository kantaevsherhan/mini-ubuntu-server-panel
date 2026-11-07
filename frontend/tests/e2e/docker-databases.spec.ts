import { expect, test } from '@playwright/test'

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    sessionStorage.setItem('access_token', 'e2e-token')
    sessionStorage.setItem('must_change_password', 'false')
    sessionStorage.setItem('role', 'admin')
    localStorage.setItem('locale', 'ru')
  })
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    if (path.endsWith('/health')) return route.fulfill({ json: { status: 'ok' } })
    if (path.endsWith('/docker/containers'))
      return route.fulfill({
        json: [
          {
            id: 'abcdefabcdef',
            name: 'fixture',
            image: 'postgres:17',
            state: 'running',
            ports: [],
            health: '',
            created_at: '2026-11-01T12:00:00Z',
          },
        ],
      })
    if (path.endsWith('/docker/stats'))
      return route.fulfill({
        json: {
          abcdefabcdef: {
            cpu_percent: 125,
            memory_usage: 134217728,
            memory_limit: 268435456,
            network_rx: 1024,
            network_tx: 2048,
            block_read: 4096,
            block_write: 8192,
            pids: 8,
            limits: { cpus: 1.5, memory_mb: 256 },
            error: false,
          },
        },
      })
    if (path.endsWith('/databases/containers')) return route.fulfill({ json: [] })
    if (path.endsWith('/databases/overview'))
      return route.fulfill({
        json: {
          version: 'PostgreSQL 17',
          databases: ['fixture'],
          schemas: ['public'],
          schema: 'public',
          tables: [{ name: 'records', type: 'table', rows: 101, size: 8192, comment: '' }],
        },
      })
    if (path.endsWith('/databases/structure'))
      return route.fulfill({
        json: {
          type: 'table',
          columns: [
            {
              name: 'id',
              type: 'integer',
              nullable: false,
              default: '',
              primary: true,
              auto_increment: true,
              comment: '',
            },
            {
              name: 'value',
              type: 'text',
              nullable: true,
              default: '',
              primary: false,
              auto_increment: false,
              comment: '',
            },
          ],
          indexes: [],
          foreign_keys: [],
          ddl: 'CREATE TABLE records (id integer PRIMARY KEY, value text);',
        },
      })
    if (path.endsWith('/databases/select')) {
      const body = route.request().postDataJSON()
      return route.fulfill({
        json: {
          columns: ['id', 'value'],
          types: [],
          rows: [[body.offset + 1, '<script>safe</script>']],
          primary_key: ['id'],
          total: 101,
          affected: 0,
          duration_ms: 1,
          truncated: false,
        },
      })
    }
    return route.fulfill({ json: [] })
  })
})

test('Docker samples refresh at 30 seconds and can be paused', async ({ page }) => {
  let calls = 0
  page.on('request', (request) => {
    if (request.url().endsWith('/docker/stats')) calls++
  })
  await page.clock.install()
  await page.goto('/docker')
  await expect(page.getByText('125.0%')).toBeVisible()
  await expect(page.getByText('128.0 MB / 256.0 MB')).toBeVisible()
  expect(calls).toBe(1)
  await page.clock.fastForward(29000)
  expect(calls).toBe(1)
  await page.clock.fastForward(1001)
  await expect.poll(() => calls).toBe(2)
  await page
    .getByRole('button', { name: 'Автообновление включено — нажмите, чтобы приостановить' })
    .click()
  await page.clock.fastForward(60000)
  expect(calls).toBe(2)
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1)
  expect(overflow).toBe(false)
  await page.screenshot({
    path: `test-results/docker-${test.info().project.name}.png`,
    fullPage: true,
  })
})

test('Container table stays usable while stats load and scrolls without jumping back', async ({
  page,
}) => {
  // Long names and digest-only images used to wrap rows taller than the virtual scroller's
  // itemSize, which made the list snap backwards while scrolling.
  const containers = Array.from({ length: 40 }, (_, index) => ({
    id: `c${String(index).padStart(11, '0')}`,
    name: `very-long-compose-project-name-service-replica-${index}`,
    image: `sha256:${'0123456789abcdef'.repeat(4)}`,
    state: 'running',
    status: 'Up 2 hours',
    ports: ['0.0.0.0:8080->80/tcp', '[::]:8080->80/tcp'],
    health: index % 2 ? 'healthy' : '',
    created_at: '2026-11-01T12:00:00Z',
  }))
  let releaseStats: () => void = () => {}
  const statsReady = new Promise<void>((resolve) => (releaseStats = resolve))
  await page.route('**/api/v1/docker/containers', (route) => route.fulfill({ json: containers }))
  await page.route('**/api/v1/docker/stats', async (route) => {
    await statsReady
    await route.fulfill({ json: {} })
  })
  await page.goto('/docker')
  await expect(page.getByText(containers[0]!.name)).toBeVisible()
  await expect(page.locator('.p-datatable-mask')).toHaveCount(0)
  releaseStats()

  const table = page.locator('.vt').first()
  const heights = await table.evaluate((element) => {
    const expected = parseFloat(getComputedStyle(element).getPropertyValue('--vt-row'))
    const rows = [...element.querySelectorAll('.p-datatable-tbody > tr')]
    return { expected, actual: [...new Set(rows.map((row) => row.getBoundingClientRect().height))] }
  })
  expect(heights.actual).toEqual([heights.expected])

  const scroller = table.locator('.p-virtualscroller')
  const box = (await scroller.boundingBox())!
  await page.mouse.move(box.x + box.width / 3, box.y + Math.min(box.height / 2, 160))
  const positions: number[] = []
  for (let step = 0; step < 25; step++) {
    await page.mouse.wheel(0, 150)
    await page.waitForTimeout(60)
    positions.push(await scroller.evaluate((element) => element.scrollTop))
  }
  expect(positions.at(-1)!).toBeGreaterThan(0)
  for (let i = 1; i < positions.length; i++)
    expect(positions[i]!).toBeGreaterThanOrEqual(positions[i - 1]!)
  expect(
    await page.evaluate(() => {
      const main = document.querySelector('main')!
      return main.scrollWidth > main.clientWidth + 1
    }),
  ).toBe(false)
})

test('Database client connects, edits rows, escapes data and clears credentials', async ({
  page,
}) => {
  const rowRequests: unknown[] = []
  await page.route('**/api/v1/databases/rows', async (route) => {
    rowRequests.push(route.request().postDataJSON())
    await route.fulfill({ json: { affected: 1 } })
  })
  await page.goto('/databases')
  await page.getByLabel('Имя базы данных', { exact: true }).fill('fixture')
  await page.getByLabel('Пароль', { exact: true }).fill('only-in-memory')
  await page.getByRole('button', { name: 'Подключиться к БД' }).click()
  await expect(page.getByText('Подключено', { exact: true })).toBeVisible()
  await page.locator('.db-side__list').getByText('records', { exact: true }).click()
  await expect(page.getByText('<script>safe</script>')).toBeVisible()
  await expect(page.getByText('Всего:')).toBeVisible()
  await page.getByRole('button', { name: 'Изменить', exact: true }).first().click()
  const dialog = page.getByRole('dialog')
  await dialog.locator('#db-field-value').fill('changed')
  await dialog.getByRole('button', { name: 'Сохранить', exact: true }).click()
  await expect.poll(() => rowRequests.length).toBe(1)
  expect(rowRequests[0]).toMatchObject({
    action: 'update',
    table: 'records',
    keys: [{ id: 1 }],
    values: { value: 'changed' },
  })
  await page.getByRole('tab', { name: 'Структура' }).click()
  await expect(page.getByText('CREATE TABLE records')).toBeVisible()
  expect(
    await page.evaluate(() => JSON.stringify(localStorage) + JSON.stringify(sessionStorage)),
  ).not.toContain('only-in-memory')
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1)).toBe(
    false,
  )
  await page.screenshot({
    path: `test-results/databases-${test.info().project.name}.png`,
    fullPage: true,
  })
  await page.getByRole('button', { name: 'Отключиться', exact: true }).click()
  await expect(page.getByLabel('Пароль', { exact: true })).toHaveValue('')
})

test('Resource edits require confirmation and send CPU/RAM limits', async ({ page }) => {
  let saved: unknown
  await page.route('**/api/v1/docker/containers/abcdefabcdef/limits', async (route) => {
    saved = route.request().postDataJSON()
    await route.fulfill({ status: 204 })
  })
  await page.goto('/docker')
  await expect(page.getByText('125.0%')).toBeVisible()
  await expect(page.locator('.p-datatable-mask')).toHaveCount(0)
  await page.getByRole('button', { name: 'Действия', exact: true }).click()
  await page.getByRole('menuitem', { name: 'Лимиты ресурсов' }).click()
  const dialog = page.getByRole('dialog', { name: 'Лимиты ресурсов: fixture' })
  await dialog.getByLabel('Лимит CPU (ядра)').fill('1.25')
  await dialog.getByLabel('Лимит RAM (MiB)').fill('512')
  await dialog.getByRole('button', { name: 'Сохранить', exact: true }).click()
  expect(saved).toBeUndefined()
  await page
    .locator('.p-confirmdialog')
    .getByRole('button', { name: 'Сохранить', exact: true })
    .click()
  await expect.poll(() => saved).toEqual({ cpus: 1.25, memory_mb: 512 })
  await expect(page.getByText('Лимиты обновлены', { exact: true })).toBeVisible()
})
