import { test, expect, type APIRequestContext } from '@playwright/test';
import type { GameState, Task } from '../../src/client/types';

async function post(context: APIRequestContext, code: string, action: string, body = {}) {
  const response = await context.post(`/api/game/${code}/${action}`, { data: body });
  expect(response.ok(), `${action}: ${await response.text()}`).toBe(true);
  return response.json() as Promise<GameState>;
}

// Play legal public actions; no seed endpoint or private game state.
async function finishGame(facilitator: APIRequestContext, player: APIRequestContext, code: string) {
  let state = await (await facilitator.get(`/api/game/${code}`)).json() as GameState;
  for (let day = 0; day < 1500 && !state.finished; day++) {
    if (state.phase === 'retro') state = await post(facilitator, code, 'continue');
    for (const project of state.projects || []) {
      if (project.started || project.completed) continue;
      const response = await facilitator.post(`/api/game/${code}/start_project`, { data: { project_id: project.id } });
      if (response.status() === 409) break;
      expect(response.ok()).toBe(true);
      state = await response.json();
    }
    for (let step = 0; step < 2 && !state.finished; step++) {
      const team = state.teams![0];
      const member = team.members![0];
      const candidates: { task: Task; stage: string }[] = [];
      for (const stage of ['review', 'in_progress', 'ready']) {
        for (const task of team.board![stage]) {
          if (task.blocked || (member.current_coin === 'heads' && stage !== 'ready')) candidates.push({ task, stage });
          if (!task.blocked) candidates.push({ task, stage: { ready: 'in_progress', in_progress: 'review', review: 'done' }[stage]! });
        }
      }
      let moved = false;
      for (const { task, stage } of candidates) {
        const response = await player.post(`/api/game/${code}/drag`, { data: { task_id: task.id, to_stage: stage } });
        if (response.ok()) { state = await response.json(); moved = true; break; }
        expect([403, 409]).toContain(response.status());
      }
      if (!moved) break;
    }
    if (!state.finished) state = await post(facilitator, code, state.next_day_is_retro ? 'start_retro' : 'next_day');
  }
  expect(state.finished).toBe(true);
}

test('proxy accepts a lobby websocket before the browser makes any API request', async ({ page }) => {
  await page.goto('/');
  const opened = await page.evaluate(() => new Promise<boolean>(resolve => {
    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const socket = new WebSocket(`${protocol}//${location.host}/ws/lobby`);
    const timer = setTimeout(() => { socket.close(); resolve(false); }, 5000);
    socket.onopen = () => { clearTimeout(timer); socket.close(); resolve(true); };
    socket.onerror = () => { clearTimeout(timer); resolve(false); };
  }));
  expect(opened).toBe(true);
});

test('room sessions survive reload/reopen, reconnect, and produce a real report', async ({ browser, baseURL }) => {
  const facilitator = await browser.newContext({ baseURL });
  const player = await browser.newContext({ baseURL });
  const page = await facilitator.newPage();
  const participant = await player.newPage();
  await participant.addInitScript(() => {
    const NativeSocket = window.WebSocket;
    (window as any).__gameSockets = [];
    window.WebSocket = new Proxy(NativeSocket, { construct(target, args) {
      const socket = new target(...args as [string]);
      (window as any).__gameSockets.push(socket);
      return socket;
    } });
  });
  await page.goto('/start');
  await page.getByRole('button', { name: 'Создать игру', exact: true }).click();
  await page.getByRole('button', { name: 'Удалить команду Зеленая' }).click();
  await page.getByRole('button', { name: 'Удалить команду Желтая' }).click();
  await page.getByRole('button', { name: 'Подтвердить создание' }).click();
  await expect(page).toHaveURL(/\/created\/[A-Z0-9]+$/i);
  const code = new URL(page.url()).pathname.split('/').at(-1)!;
  await page.getByRole('link', { name: 'Открыть панель ведущего' }).click();
  await expect(page.getByRole('heading', { name: 'Панель ведущего' })).toBeVisible();
  await participant.goto(`/joining/${code}`);
  await participant.getByLabel('Ваш никнейм').fill('Browser player');
  await participant.getByRole('button', { name: 'Войти в игру' }).click();
  await expect(participant).toHaveURL(new RegExp(`/game/${code}$`));
  await expect(participant.getByText('Роль: Игрок (Browser player)', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Запустить проект', exact: true }).click();
  await page.getByRole('button', { name: 'Старт игры', exact: true }).click();
  const take = participant.getByRole('button', { name: 'Взять задачу', exact: true }).first();
  await expect(take).toBeEnabled();
  await take.focus();
  await participant.keyboard.press('Enter');
  await expect(participant.getByText('Ответственный: Browser player').first()).toBeVisible();
  await participant.reload();
  await expect(participant.getByText('Роль: Игрок (Browser player)', { exact: true })).toBeVisible();
  await expect(participant.getByText('Подключено', { exact: true })).toBeVisible();
  await participant.evaluate(() => (window as any).__gameSockets.at(-1).close());
  await expect(participant.getByText('Переподключение · обновляем по HTTP', { exact: true })).toBeVisible();
  await expect(participant.getByText('Подключено', { exact: true })).toBeVisible();
  const stored = await facilitator.storageState();
  await facilitator.close();
  const reopened = await browser.newContext({ baseURL, storageState: stored });
  const restored = await reopened.newPage();
  await restored.goto(`/game/${code}?player_id=forged`);
  await expect(restored.getByRole('heading', { name: 'Панель ведущего' })).toBeVisible();
  await finishGame(reopened.request, player.request, code);
  await restored.reload();
  await restored.getByRole('link', { name: 'Игра завершена · открыть отчёт' }).click();
  await expect(restored.getByRole('heading', { name: 'Как менялся поток' })).toBeVisible();
  await expect(restored.getByRole('table', { name: 'Данные CFD и метрик по дням' }).locator('tbody tr').first()).toBeVisible();
  const downloadEvent = restored.waitForEvent('download');
  await restored.getByRole('link', { name: 'Скачать CSV' }).click();
  const download = await downloadEvent;
  expect(download.suggestedFilename()).toMatch(/\.csv$/);
  expect(await download.failure()).toBeNull();
  await restored.goto('/demo');
  await expect(restored.getByText('Демонстрационный отчёт', { exact: true })).toBeVisible();
  await expect(restored.getByText(/фиксированный учебный пример/)).toBeVisible();
  await reopened.close();
  await player.close();
});
