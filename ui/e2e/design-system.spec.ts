import { expect, test, type Page } from '@playwright/test';
import { fixtureCluster, fixtureMe, paged } from '../src/test/fixtures';

// Match the deployed Go middleware, not just Vite preview's permissive headers.
test.beforeEach(async ({ page }) => {
  await page.route('**/ui/**', async (route) => {
    if (route.request().resourceType() !== 'document') return route.continue();
    const response = await route.fetch();
    await route.fulfill({
      response,
      headers: {
        ...response.headers(),
        'content-security-policy': "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; frame-ancestors 'none'",
      },
    });
  });
});

async function tokenColor(page: Page, token: string) {
  return page.evaluate((name) => {
    const swatch = document.createElement('span');
    swatch.style.backgroundColor = `var(${name})`;
    document.body.append(swatch);
    const color = getComputedStyle(swatch).backgroundColor;
    swatch.remove();
    return color;
  }, token);
}

test('login ships the real DS controls, utilities and local fonts', async ({ page }, testInfo) => {
  const externalRequests: string[] = [];
  page.on('request', (request) => {
    if (request.url().startsWith('http') && new URL(request.url()).hostname !== '127.0.0.1') {
      externalRequests.push(request.url());
    }
  });
  await page.route('**/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === '/v1/auth/me') {
      await route.fulfill({ status: 401, json: { detail: 'Sign in required' } });
    } else if (path === '/v1/auth/config') {
      await route.fulfill({ json: { oidc: { enabled: false } } });
    } else {
      await route.fulfill({ status: 401, json: { detail: 'Invalid credentials.' } });
    }
  });
  await page.goto('/ui/login');

  const submit = page.getByRole('button', { name: 'Sign in', exact: true });
  const password = page.getByLabel('Password', { exact: true });
  await expect(submit).toHaveAttribute('data-slot', 'button');
  await expect(submit).toHaveCSS('height', '48px');
  await expect(submit).toHaveCSS('border-radius', '8px');
  await expect(submit).toHaveCSS('background-color', await tokenColor(page, '--color-primary'));
  await expect(page.locator('[data-slot="input-container"]').first()).toHaveCSS('height', '48px');
  await expect(page.locator('[data-slot="input-container"]').last()).toHaveCSS('flex-direction', 'row');
  await expect(page.locator('label[for="password"]')).toHaveCSS('text-transform', 'none');

  await page.getByRole('button', { name: 'Show password', exact: true }).click();
  await expect(password).toHaveAttribute('type', 'text');
  await page.getByRole('button', { name: 'Hide password', exact: true }).click();
  await expect(password).toHaveAttribute('type', 'password');
  await submit.click();
  await expect(page.getByRole('alert')).toContainText('Username and password required.');

  await page.evaluate(() => document.fonts.ready);
  expect(await page.evaluate(() => [...document.fonts].some((font) =>
    font.family.includes('Rethink Sans') && font.status === 'loaded',
  ))).toBe(true);
  expect(externalRequests).toEqual([]);
  await page.screenshot({ path: testInfo.outputPath('login.png'), fullPage: true });
});

test('inventory keeps native filters usable without overriding DS controls', async ({ page }, testInfo) => {
  await page.route('**/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === '/v1/auth/me') {
      await route.fulfill({ json: fixtureMe });
    } else if (path === '/v1/clusters') {
      await route.fulfill({ json: { ...paged([fixtureCluster]), next_cursor: 'next-page' } });
    } else {
      throw new Error(`Unexpected API call: ${route.request().url()}`);
    }
  });
  await page.goto('/ui/clusters');
  await expect(page.getByRole('link', { name: 'Prod EU West', exact: true })).toBeVisible();

  const search = page.getByRole('searchbox', { name: 'Name', exact: true });
  const searchContainer = page.locator('[data-slot="search"]');
  await expect(searchContainer).toHaveCSS('flex-direction', 'row');
  await expect(searchContainer).toHaveCSS('height', '40px');
  await expect(search).toHaveCSS('border-top-width', '0px');
  await search.fill('prod');
  await page.getByRole('button', { name: 'Clear search', exact: true }).click();
  await expect(search).toHaveValue('');
  await expect(search).toBeFocused();

  const nativeSelect = page.getByRole('combobox', { name: 'Status', exact: true });
  await expect(nativeSelect).toHaveCSS('border-top-width', '1px');
  await expect(nativeSelect).toHaveCSS('background-color', await tokenColor(page, '--color-background-darker'));
  const prev = page.getByRole('button', { name: /prev/i });
  await expect(prev).toBeDisabled();
  await expect(prev).toHaveCSS('height', '32px');
  await expect(prev).toHaveCSS('background-color', await tokenColor(page, '--color-disabled'));

  const clustersLink = page.getByRole('navigation').getByRole('link', { name: 'Clusters', exact: true });
  await page.keyboard.press('Tab');
  await clustersLink.focus();
  await expect(clustersLink).toHaveCSS('outline-style', 'solid');
  await expect(clustersLink).toHaveCSS('outline-width', '2px');

  const toggle = page.getByRole('button', { name: 'Collapse navigation', exact: true });
  await toggle.click();
  await expect(page.getByRole('button', { name: 'Expand navigation', exact: true })).toHaveAttribute('aria-expanded', 'false');
  await page.getByRole('button', { name: 'Expand navigation', exact: true }).click();
  await page.screenshot({ path: testInfo.outputPath('clusters.png'), fullPage: true });
});
