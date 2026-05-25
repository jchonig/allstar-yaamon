import { test, expect } from '@playwright/test';
import { loginAsAdmin } from './fixtures';

test.describe('Admin — Nodes page', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/admin/nodes');
  });

  test('page loads', async ({ page }) => {
    await expect(page).toHaveURL('/admin/nodes');
    await expect(page.locator('body')).toBeVisible();
  });

  test('shows seeded test node', async ({ page }) => {
    await expect(page.locator('body')).toContainText('Test Node');
  });

  test('Add Node button is present', async ({ page }) => {
    await expect(page.locator('button', { hasText: /Add Node/i })).toBeVisible();
  });
});

test.describe('Admin — Users page', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/admin/users');
  });

  test('page loads', async ({ page }) => {
    await expect(page).toHaveURL('/admin/users');
    await expect(page.locator('body')).toBeVisible();
  });

  test('shows seeded admin user', async ({ page }) => {
    await expect(page.locator('body')).toContainText('admin');
  });

  test('Add User button is present', async ({ page }) => {
    await expect(page.locator('button', { hasText: /Add User/i })).toBeVisible();
  });
});

test.describe('Admin — nav dropdown', () => {
  test('backup link present in dropdown', async ({ page }) => {
    await loginAsAdmin(page);
    await page.locator('.dropdown-toggle').click();
    const backupLink = page.locator('a[href="/admin/backup"]');
    await expect(backupLink).toBeVisible();
    await backupLink.click();
    await expect(page).toHaveURL('/admin/backup');
  });
});
