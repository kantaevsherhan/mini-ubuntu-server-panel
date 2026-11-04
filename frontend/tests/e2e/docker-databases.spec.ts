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
    if (path.endsWith('/databases/browse')) {
      const body = route.request().postDataJSON()
      return route.fulfill({
        json: {
          tables: ['records'],
          columns: body.table ? ['id', 'value'] : [],
          rows: body.table ? [{ id: body.offset + 1, value: '<script>safe</script>' }] : [],
          has_more: body.offset === 0,
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

test('Database client connects, pages, escapes data and clears credentials', async ({ page }) => {
  await page.goto('/databases')
  await page.getByLabel('Имя базы данных', { exact: true }).fill('fixture')
  await page.getByLabel('Пароль', { exact: true }).fill('only-in-memory')
  await page.getByRole('button', { name: 'Подключиться к БД' }).click()
  await expect(page.getByText('Подключено', { exact: true })).toBeVisible()
  await page.getByLabel('Таблица / коллекция').click()
  await page.getByRole('option', { name: 'records', exact: true }).click()
  await expect(page.getByText('<script>safe</script>')).toBeVisible()
  await page.getByRole('button', { name: 'Далее', exact: true }).click()
  await expect(page.getByText('Записи: 101–101')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Далее', exact: true })).toBeDisabled()
  expect(
    await page.evaluate(() => JSON.stringify(localStorage) + JSON.stringify(sessionStorage)),
  ).not.toContain('only-in-memory')
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1)).toBe(
    false,
  )
  await expect(page.locator('.p-datatable-mask')).toHaveCount(0)
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
