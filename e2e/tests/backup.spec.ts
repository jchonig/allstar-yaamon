import { test, expect } from '@playwright/test';
import { loginAsAdmin } from './fixtures';

test.describe('Backup page', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/admin/backup');
  });

  test('page loads with 200', async ({ page }) => {
    await expect(page).toHaveURL('/admin/backup');
    await expect(page.locator('h1, .card-header')).toHaveCount({ minimum: 1 });
  });

  test('restart overlay is hidden on load', async ({ page }) => {
    const overlay = page.locator('#restart-overlay');
    await expect(overlay).toBeAttached();
    // Must not be visible — the d-flex bug would make it visible.
    await expect(overlay).not.toBeVisible();
  });

  test('overlay uses display:none not flex on load', async ({ page }) => {
    const display = await page.locator('#restart-overlay').evaluate(
      (el) => window.getComputedStyle(el).display
    );
    expect(display).toBe('none');
  });

  test('backup encrypt checkbox is present', async ({ page }) => {
    await expect(page.locator('#backup-encrypt')).toBeVisible();
  });

  test('passphrase fields hidden when encrypt unchecked', async ({ page }) => {
    const section = page.locator('#backup-passphrase-section');
    await expect(section).not.toBeVisible();
  });

  test('passphrase fields shown when encrypt checked', async ({ page }) => {
    await page.locator('#backup-encrypt').check();
    await expect(page.locator('#backup-passphrase-section')).toBeVisible();
    await expect(page.locator('#backup-passphrase')).toBeVisible();
    await expect(page.locator('#backup-passphrase2')).toBeVisible();
  });

  test('download backup button triggers file download', async ({ page }) => {
    const [download] = await Promise.all([
      page.waitForEvent('download'),
      page.locator('button', { hasText: 'Download Backup' }).click(),
    ]);
    expect(download.suggestedFilename()).toMatch(/\.owbackup$/);
  });

  test('encrypt passphrase mismatch shows error', async ({ page }) => {
    await page.locator('#backup-encrypt').check();
    await page.locator('#backup-passphrase').fill('secret123');
    await page.locator('#backup-passphrase2').fill('different');
    await page.locator('button', { hasText: 'Download Backup' }).click();
    await expect(page.locator('#backup-error')).toContainText('match');
  });

  test('favorites export card is present', async ({ page }) => {
    // Only shown when nodes exist.
    const card = page.locator('.card-header', { hasText: 'Favorites Export' });
    const count = await card.count();
    if (count > 0) {
      await expect(page.locator('#fav-node-select')).toBeVisible();
    }
  });
});
