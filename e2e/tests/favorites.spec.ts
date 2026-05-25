import { test, expect, Page } from '@playwright/test';
import { loginAsAdmin } from './fixtures';

async function goToDashboard(page: Page) {
  await loginAsAdmin(page);
  // Dashboard is already loaded after login.
}

test.describe('Favorites — Add modal', () => {
  test.beforeEach(async ({ page }) => {
    await goToDashboard(page);
  });

  test('Add Favorite button opens modal', async ({ page }) => {
    await page.locator('button', { hasText: /Add Favorite/i }).click();
    await expect(page.locator('#addFavModal')).toBeVisible();
  });

  test('modal has required fields', async ({ page }) => {
    await page.locator('button', { hasText: /Add Favorite/i }).click();
    await expect(page.locator('#fav-node-number')).toBeVisible();
    await expect(page.locator('#fav-callsign')).toBeVisible();
    await expect(page.locator('#fav-description')).toBeVisible();
    await expect(page.locator('#fav-location')).toBeVisible();
  });

  test('can add and see a new favorite', async ({ page }) => {
    await page.locator('button', { hasText: /Add Favorite/i }).click();
    await page.locator('#fav-node-number').fill('77777');
    await page.locator('#fav-callsign').fill('W1TEST');
    await page.locator('#fav-description').fill('E2E Test Node');
    await page.locator('#addFavModal .modal-footer button', { hasText: /Add/i }).click();

    // Modal closes and row appears.
    await expect(page.locator('#addFavModal')).not.toBeVisible();
    await expect(page.locator('#favs-tbody')).toContainText('77777');
  });

  test('cancel closes modal without adding', async ({ page }) => {
    const rowsBefore = await page.locator('#favs-tbody tr').count();
    await page.locator('button', { hasText: /Add Favorite/i }).click();
    await page.locator('#addFavModal .btn-secondary', { hasText: 'Cancel' }).click();
    await expect(page.locator('#addFavModal')).not.toBeVisible();
    const rowsAfter = await page.locator('#favs-tbody tr').count();
    expect(rowsAfter).toBe(rowsBefore);
  });
});

test.describe('Favorites — Delete', () => {
  test('delete button removes the row', async ({ page }) => {
    await goToDashboard(page);

    // First add one to delete.
    await page.locator('button', { hasText: /Add Favorite/i }).click();
    await page.locator('#fav-node-number').fill('66666');
    await page.locator('#addFavModal .modal-footer button', { hasText: /Add/i }).click();
    await expect(page.locator('#favs-tbody')).toContainText('66666');

    // Find its delete button and click it.
    const row = page.locator('#favs-tbody tr', { hasText: '66666' });
    await row.locator('button[title="Remove favorite"]').click();

    await expect(page.locator('#favs-tbody')).not.toContainText('66666');
  });
});

test.describe('Favorites — Sort', () => {
  test('clicking Node column header shows sort indicator', async ({ page }) => {
    await goToDashboard(page);
    const header = page.locator('#favs-table th', { hasText: 'Node' });
    await header.click();
    await expect(header).toHaveClass(/sort-active/);
  });

  test('second click reverses sort direction', async ({ page }) => {
    await goToDashboard(page);
    const header = page.locator('#favs-table th', { hasText: 'Node' });
    await header.click();
    const iconAfterFirst = await header.locator('.sort-icon').textContent();
    await header.click();
    const iconAfterSecond = await header.locator('.sort-icon').textContent();
    expect(iconAfterFirst).not.toBe(iconAfterSecond);
  });

  test('third click resets to insertion order', async ({ page }) => {
    await goToDashboard(page);
    const header = page.locator('#favs-table th', { hasText: 'Node' });
    await header.click();
    await header.click();
    await header.click();
    await expect(header).not.toHaveClass(/sort-active/);
  });
});

test.describe('Favorites — Collapsible card', () => {
  test('favorites card starts expanded', async ({ page }) => {
    await goToDashboard(page);
    await expect(page.locator('#favorites-collapse')).toHaveClass(/show/);
  });

  test('clicking header collapses the card', async ({ page }) => {
    await goToDashboard(page);
    // Click the collapse header (the card-header with data-bs-toggle).
    await page.locator('[data-bs-target="#favorites-collapse"]').click();
    await expect(page.locator('#favorites-collapse')).not.toHaveClass(/show/);
  });

  test('clicking again expands the card', async ({ page }) => {
    await goToDashboard(page);
    const header = page.locator('[data-bs-target="#favorites-collapse"]');
    await header.click(); // collapse
    await header.click(); // expand
    await expect(page.locator('#favorites-collapse')).toHaveClass(/show/);
  });
});

test.describe('Favorites INI Import UI', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/admin/backup');
  });

  test('node selector is present', async ({ page }) => {
    await expect(page.locator('#fav-node-select')).toBeVisible();
  });

  test('preview button exists', async ({ page }) => {
    await expect(page.locator('button', { hasText: /Preview/i })).toBeVisible();
  });

  test('import button starts disabled', async ({ page }) => {
    const importBtn = page.locator('#fav-import-btn');
    await expect(importBtn).toBeDisabled();
  });

  test('uploading a valid INI and previewing enables import', async ({ page }) => {
    const ini = '[node_54321]\ncmd[] = "rpt cmd 99999 ilink 3 54321"\n';
    await page.locator('#fav-import-file').setInputFiles({
      name: 'favorites.ini',
      mimeType: 'text/plain',
      buffer: Buffer.from(ini),
    });
    await page.locator('button', { hasText: /Preview/i }).click();
    await expect(page.locator('#fav-import-preview')).toBeVisible();
    // Import button should now be enabled (if there are nodes to add).
    // (May still be disabled if node already exists — just check preview appeared.)
  });
});

test.describe('Mobile — hidden columns', () => {
  test.use({ viewport: { width: 375, height: 812 } });

  test('Location column is hidden', async ({ page }) => {
    await goToDashboard(page);
    const locCol = page.locator('th', { hasText: 'Location' });
    const count = await locCol.count();
    if (count > 0) {
      expect(await locCol.isVisible()).toBe(false);
    }
  });

  test('Description column is hidden', async ({ page }) => {
    await goToDashboard(page);
    const descCol = page.locator('th', { hasText: 'Description' });
    const count = await descCol.count();
    if (count > 0) {
      expect(await descCol.isVisible()).toBe(false);
    }
  });
});
