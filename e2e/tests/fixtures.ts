import { Page } from '@playwright/test';

export const ADMIN_USER = 'admin';
export const ADMIN_PASS = process.env.ADMIN_PASSWORD ?? 'testpassword';
export const VIEWER_USER = 'viewer';
export const VIEWER_PASS = process.env.VIEWER_PASSWORD ?? 'viewerpassword';

export async function loginAs(page: Page, username: string, password: string): Promise<void> {
  await page.goto('/login');
  await page.fill('#username', username);
  await page.fill('#password', password);
  await page.click('button[type="submit"]');
  await page.waitForURL('/dashboard**');
}

export async function loginAsAdmin(page: Page): Promise<void> {
  return loginAs(page, ADMIN_USER, ADMIN_PASS);
}

export async function loginAsViewer(page: Page): Promise<void> {
  return loginAs(page, VIEWER_USER, VIEWER_PASS);
}
