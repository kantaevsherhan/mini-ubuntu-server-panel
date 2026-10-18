import { expect, test } from '@playwright/test'

const metrics = {
  range: 'day',
  points: [
    {
      sampled_at: '2026-07-24T12:00:00Z',
      cpu_percent: 42,
      memory_percent: 58,
      memory_used_bytes: 4_982_784,
      memory_total_bytes: 8_589_934_592,
    },
  ],
}

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    sessionStorage.setItem('access_token', 'e2e-token')
    sessionStorage.setItem('must_change_password', 'false')
    sessionStorage.setItem('username', 'admin')
    sessionStorage.setItem('role', 'admin')
    localStorage.setItem('locale', 'ru')
  })
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/dashboard') {
      await route.fulfill({
        json: {
          hostname: 'e2e-server',
          panel_users: 3,
          pending_notifications: 1,
          status: 'online',
        },
      })
      return
    }
    if (path === '/api/v1/metrics/history') {
      await route.fulfill({ json: metrics })
      return
    }
    if (path === '/api/v1/health') {
      await route.fulfill({ json: { status: 'ok', version: 'v1.0.0' } })
      return
    }
    if (path === '/api/v1/settings/overview') {
      await route.fulfill({
        json: {
          hostname: 'e2e-server',
          version: 'v1.0.0',
          go_version: 'go1.25',
          os: 'linux',
          architecture: 'amd64',
          data_dir: '/var/lib/mini-ubuntu-server',
          log_dir: '/var/log/mini-ubuntu-server',
          database_size_bytes: 4096,
          metric_samples: 10,
          audit_events: 2,
          active_sessions: 1,
          allowed_directories: [],
        },
      })
      return
    }
    if (path === '/api/v1/audit') {
      await route.fulfill({
        json: [
          {
            id: 1,
            actor_user_id: 1,
            action: 'auth.login',
            target_type: 'user',
            target_id: 'admin',
            details: '{}',
            ip_address: '127.0.0.1',
            created_at: '2026-07-24T12:00:00Z',
          },
        ],
      })
      return
    }
    if (path === '/api/v1/notifications/rules') {
      await route.fulfill({ json: [] })
      return
    }
    if (path === '/api/v1/telegram/recipients') {
      await route.fulfill({ json: [] })
      return
    }
    if (path === '/api/v1/notifications/history') {
      await route.fulfill({ json: [] })
      return
    }
    await route.fulfill({ status: 404, json: { error: 'not_found' } })
  })
})

test('dashboard and settings adapt without horizontal page overflow', async ({
  page,
}, testInfo) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Состояние сервера' })).toBeVisible()
  await expect(page.getByText('e2e-server')).toBeVisible()
  await expect(page.getByText('42%')).toBeVisible()

  const mobile = testInfo.project.name.startsWith('mobile')
  const menuButton = page.getByRole('button', { name: 'Открыть навигацию' })
  if (mobile) {
    await expect(menuButton).toBeVisible()
    await menuButton.click()
    await page.getByText('Настройки', { exact: true }).last().click()
  } else {
    await expect(menuButton).toBeHidden()
    await page.getByText('Настройки', { exact: true }).first().click()
  }
  await expect(page.getByRole('heading', { name: 'Настройки' })).toBeVisible()
  await expect(page.getByRole('tab', { name: 'Интерфейс' })).toBeVisible()
  const overflows = await page.evaluate(
    () => document.documentElement.scrollWidth > window.innerWidth + 1,
  )
  expect(overflows).toBe(false)
})

test('server errors are shown through PrimeVue Toast', async ({ page }) => {
  await page.route('**/api/v1/dashboard', (route) =>
    route.fulfill({ status: 500, json: { error: 'internal_error' } }),
  )
  await page.goto('/')
  await expect(page.getByText('Ошибка запроса')).toBeVisible()
  await expect(page.getByText('Код ошибки: internal_error')).toBeVisible()
})

test('command palette navigates with the keyboard', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Состояние сервера' })).toBeVisible()
  await page.keyboard.press('Control+k')
  await page.getByPlaceholder('Страница или действие…').fill('настр')
  await page.keyboard.press('Enter')
  await expect(page.getByRole('heading', { name: 'Настройки' })).toBeVisible()
})

test('header shows offline state when the server is unreachable', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('status', { name: 'Онлайн' })).toBeVisible()
  await page.route('**/api/v1/metrics/history**', (route) => route.abort('connectionrefused'))
  await page.getByRole('button', { name: 'Неделя' }).click()
  await expect(page.getByRole('status', { name: 'Нет связи' })).toBeVisible()
})

test('login asks for the second factor before opening the panel', async ({ page }) => {
  let codeSent = ''
  await page.addInitScript(() => sessionStorage.clear())
  await page.route('**/api/v1/auth/login', (route) =>
    route.fulfill({ json: { mfa_required: true, mfa_token: 'challenge' } }),
  )
  await page.route('**/api/v1/auth/login/mfa', async (route) => {
    codeSent = route.request().postDataJSON().code
    await route.fulfill({
      json: { access_token: 'e2e-token', username: 'admin', role: 'admin' },
    })
  })
  await page.goto('/login')
  await page.locator('input#password, #password input').first().fill('admin-password')
  await page.getByRole('button', { name: 'Войти' }).click()
  await expect(page.getByText('Подтверждение входа')).toBeVisible()
  await expect(page.locator('.login-otp input').first()).toBeFocused()
  await page.keyboard.type('123456')
  await expect(page.getByRole('heading', { name: 'Состояние сервера' })).toBeVisible()
  expect(codeSent).toBe('123456')
})

test('apt updates page lists packages and starts an upgrade', async ({ page }) => {
  let upgradeBody: unknown = 'not-called'
  await page.route('**/api/v1/apt', (route) =>
    route.fulfill({
      json: {
        packages: [
          {
            name: 'openssl',
            current: '3.0.13-0ubuntu3.4',
            candidate: '3.0.13-0ubuntu3.5',
            origin: 'Ubuntu:24.04/noble-security',
            security: true,
            new: false,
          },
          {
            name: 'tzdata',
            current: '2024a-3ubuntu1',
            candidate: '2024b-0ubuntu0.24.04',
            origin: 'Ubuntu:24.04/noble-updates',
            security: false,
            new: false,
          },
        ],
        security_count: 1,
        last_refresh: '2026-07-24T11:00:00Z',
        reboot_required: false,
        reboot_packages: [],
        auto_upgrades: { installed: true, enabled: true },
        job: { state: 'idle', log: [] },
      },
    }),
  )
  await page.route('**/api/v1/apt/upgrade', async (route) => {
    upgradeBody = route.request().postData()
    await route.fulfill({ status: 202, body: '' })
  })
  await page.route('**/api/v1/apt/job', (route) =>
    route.fulfill({
      json: { state: 'running', operation: 'upgrade', log: ['Unpacking openssl ...'] },
    }),
  )
  await page.goto('/updates')
  await expect(page.getByRole('heading', { name: 'Обновления пакетов' })).toBeVisible()
  await expect(page.getByText('openssl', { exact: true })).toBeVisible()
  // Relative time must use the Russian moment locale, not fall back to English.
  await expect(page.getByText(/назад$/)).toBeVisible()
  await page.getByRole('button', { name: 'Обновить всё' }).click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Подтвердить' }).click()
  await expect(page.getByText('Unpacking openssl ...')).toBeVisible()
  expect(upgradeBody).toBeNull()
  const overflows = await page.evaluate(
    () => document.documentElement.scrollWidth > window.innerWidth + 1,
  )
  expect(overflows).toBe(false)
})

test('audit and notification modules render real responsive pages', async ({ page }) => {
  await page.goto('/audit')
  await expect(page.getByRole('heading', { name: 'Аудит' })).toBeVisible()
  await expect(page.getByText('auth.login')).toBeVisible()

  await page.goto('/notifications')
  await expect(page.getByRole('heading', { name: 'Уведомления' })).toBeVisible()
  await expect(page.getByText('Правила уведомлений')).toBeVisible()
  await expect(page.getByText('История доставки', { exact: true })).toBeVisible()

  const overflows = await page.evaluate(
    () => document.documentElement.scrollWidth > window.innerWidth + 1,
  )
  expect(overflows).toBe(false)
})
