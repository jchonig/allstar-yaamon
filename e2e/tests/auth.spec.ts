import { test, expect } from '@playwright/test';
import { ADMIN_USER, ADMIN_PASS, VIEWER_USER, VIEWER_PASS, loginAsAdmin } from './fixtures';

test.describe('Login page', () => {
  test('renders username and password fields', async ({ page }) => {
    await page.goto('/login');
    await expect(page.locator('#username')).toBeVisible();
    await expect(page.locator('#password')).toBeVisible();
    await expect(page.locator('button[type="submit"]')).toBeVisible();
  });

  test('wrong password shows error', async ({ page }) => {
    await page.goto('/login');
    await page.fill('#username', ADMIN_USER);
    await page.fill('#password', 'definitely-wrong');
    await page.click('button[type="submit"]');
    await expect(page.locator('.alert-danger')).toBeVisible();
    // Should remain on login page.
    await expect(page).toHaveURL('/login');
  });

  test('unknown user shows error', async ({ page }) => {
    await page.goto('/login');
    await page.fill('#username', 'nobody');
    await page.fill('#password', 'password');
    await page.click('button[type="submit"]');
    await expect(page.locator('.alert-danger')).toBeVisible();
  });

  test('correct credentials redirect to dashboard', async ({ page }) => {
    await page.goto('/login');
    await page.fill('#username', ADMIN_USER);
    await page.fill('#password', ADMIN_PASS);
    await page.click('button[type="submit"]');
    await expect(page).toHaveURL('/dashboard');
  });
});

test.describe('Session', () => {
  test('logout redirects to login', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/logout');
    await expect(page).toHaveURL('/login');
  });

  test('protected page redirects to login when unauthenticated', async ({ page }) => {
    await page.goto('/dashboard');
    await expect(page).toHaveURL(/\/login/);
  });

  test('viewer can access dashboard', async ({ page }) => {
    await page.goto('/login');
    await page.fill('#username', VIEWER_USER);
    await page.fill('#password', VIEWER_PASS);
    await page.click('button[type="submit"]');
    await expect(page).toHaveURL('/dashboard');
  });

  test('viewer is blocked from admin pages', async ({ page }) => {
    await page.goto('/login');
    await page.fill('#username', VIEWER_USER);
    await page.fill('#password', VIEWER_PASS);
    await page.click('button[type="submit"]');

    const response = await page.goto('/admin/nodes');
    expect(response?.status()).toBe(403);
  });
});
