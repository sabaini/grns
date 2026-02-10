import { expect, test } from '@playwright/test';

test('task list to detail smoke flow', async ({ page, request }) => {
  const title = `UI smoke ${Date.now()}`;

  const create = await request.post('/v1/projects/gr/tasks', {
    data: { title },
  });
  expect(create.ok()).toBeTruthy();

  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Task list' })).toBeVisible();
  await expect(page.getByText(title)).toBeVisible();

  await page.getByText(title).click();
  await expect(page.getByRole('heading', { name: 'Task detail' })).toBeVisible();
  await expect(page.getByText(title)).toBeVisible();
});
