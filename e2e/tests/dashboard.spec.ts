import { test, expect } from '@playwright/test';
import { loginAsAdmin, loginAsViewer } from './fixtures';

test.describe('Dashboard', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('shows the node name and number', async ({ page }) => {
    await expect(page.locator('h5')).toContainText('Test Node');
    await expect(page.locator('code.text-muted')).toContainText('99999');
  });

  test('AMI badge is present', async ({ page }) => {
    await expect(page.locator('#ami-badge')).toBeVisible();
  });

  test('favorites card is present', async ({ page }) => {
    // The favorites card should exist (may be empty but the card itself is there).
    await expect(page.locator('.card')).toHaveCount({ minimum: 1 });
  });

  test('active links section exists in DOM', async ({ page }) => {
    // Starts hidden (display:none) when no links; element must exist.
    const section = page.locator('#active-links-section');
    await expect(section).toBeAttached();
  });

  test('admin dropdown shows admin links', async ({ page }) => {
    await page.locator('.dropdown-toggle').click();
    await expect(page.locator('a[href="/admin/nodes"]')).toBeVisible();
    await expect(page.locator('a[href="/admin/users"]')).toBeVisible();
    await expect(page.locator('a[href="/admin/backup"]')).toBeVisible();
  });

  test('viewer dropdown does not show admin links', async ({ page }) => {
    await page.close();
    const newPage = await page.context().newPage();
    await loginAsViewer(newPage);
    await newPage.locator('.dropdown-toggle').click();
    await expect(newPage.locator('a[href="/admin/nodes"]')).not.toBeVisible();
  });
});

test.describe('Dashboard — mobile viewport', () => {
  test.use({ viewport: { width: 375, height: 812 } });

  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('page loads without horizontal scroll', async ({ page }) => {
    const scrollWidth = await page.evaluate(() => document.documentElement.scrollWidth);
    const clientWidth = await page.evaluate(() => document.documentElement.clientWidth);
    expect(scrollWidth).toBeLessThanOrEqual(clientWidth + 2);
  });

  test('description column is hidden on mobile', async ({ page }) => {
    // The d-none d-md-table-cell class hides on xs screens.
    const descCol = page.locator('th', { hasText: 'Description' });
    // May not exist at all or be hidden — just check it's not causing overflow.
    const count = await descCol.count();
    if (count > 0) {
      const visible = await descCol.isVisible();
      expect(visible).toBe(false);
    }
  });
});
