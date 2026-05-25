import { test, expect } from '@playwright/test';
import { loginAsAdmin } from './fixtures';

test.describe('Admin — User modals', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/admin/users');
  });

  test('Add User modal opens', async ({ page }) => {
    await page.locator('button', { hasText: /Add User/i }).click();
    await expect(page.locator('#userModal')).toBeVisible();
    await expect(page.locator('#user-username')).toBeVisible();
    await expect(page.locator('#user-permission')).toBeVisible();
    await expect(page.locator('#user-password')).toBeVisible();
  });

  test('can create a user and see them in the table', async ({ page }) => {
    await page.locator('button', { hasText: /Add User/i }).click();
    await page.locator('#user-username').fill('e2etestuser');
    await page.locator('#user-permission').selectOption('readonly');
    await page.locator('#user-password').fill('TestPassword1!');
    await page.locator('#user-save-btn').click();

    await expect(page.locator('#userModal')).not.toBeVisible();
    await expect(page.locator('#users-tbody')).toContainText('e2etestuser');
  });

  test('Edit button opens modal with current user data', async ({ page }) => {
    const row = page.locator('#users-tbody tr', { hasText: 'viewer' });
    await row.locator('button', { hasText: 'Edit' }).click();
    await expect(page.locator('#userModal')).toBeVisible();
    // Username field should be disabled when editing.
    await expect(page.locator('#user-username')).toBeDisabled();
    // Password field should be optional (blank = keep existing).
    await expect(page.locator('#pw-optional')).toBeVisible();
  });

  test('Delete modal shows username', async ({ page }) => {
    // Create a user to delete.
    await page.locator('button', { hasText: /Add User/i }).click();
    await page.locator('#user-username').fill('deleteme');
    await page.locator('#user-permission').selectOption('readonly');
    await page.locator('#user-password').fill('TestPassword1!');
    await page.locator('#user-save-btn').click();
    await expect(page.locator('#users-tbody')).toContainText('deleteme');

    const row = page.locator('#users-tbody tr', { hasText: 'deleteme' });
    await row.locator('button', { hasText: 'Delete' }).click();
    await expect(page.locator('#deleteModal')).toBeVisible();
    await expect(page.locator('#delete-user-name')).toContainText('deleteme');
  });

  test('confirming delete removes the user', async ({ page }) => {
    // Create then delete.
    await page.locator('button', { hasText: /Add User/i }).click();
    await page.locator('#user-username').fill('tempuser');
    await page.locator('#user-permission').selectOption('readonly');
    await page.locator('#user-password').fill('TestPassword1!');
    await page.locator('#user-save-btn').click();
    await expect(page.locator('#users-tbody')).toContainText('tempuser');

    const row = page.locator('#users-tbody tr', { hasText: 'tempuser' });
    await row.locator('button', { hasText: 'Delete' }).click();
    await page.locator('#deleteModal button', { hasText: 'Delete' }).click();
    await expect(page.locator('#users-tbody')).not.toContainText('tempuser');
  });

  test('password strength bar appears while typing', async ({ page }) => {
    await page.locator('button', { hasText: /Add User/i }).click();
    await page.locator('#user-password').fill('weak');
    await expect(page.locator('#pw-strength-bar')).toBeVisible();
  });

  test('cancel does not create a user', async ({ page }) => {
    const countBefore = await page.locator('#users-tbody tr').count();
    await page.locator('button', { hasText: /Add User/i }).click();
    await page.locator('#user-username').fill('shouldnotappear');
    await page.locator('#userModal .btn-secondary', { hasText: 'Cancel' }).click();
    await expect(page.locator('#userModal')).not.toBeVisible();
    const countAfter = await page.locator('#users-tbody tr').count();
    expect(countAfter).toBe(countBefore);
  });
});
