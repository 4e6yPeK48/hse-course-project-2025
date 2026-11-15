import { test } from 'node:test';
import assert from 'node:assert/strict';
import { newerState, startGameConnection } from '../src/client/lib/connection';
import type { GameState } from '../src/client/types';

test('HTTP and socket states only replace older revisions', () => {
  const current = { revision: 5 } as GameState;
  assert.equal(newerState(current, { revision: 4 } as GameState), current);
  assert.equal(newerState(current, { revision: 5 } as GameState), current);
  assert.equal(newerState(current, { revision: 6 } as GameState).revision, 6);
});

test('reconnect backs off, caps delay, ignores old events, and cleans up', () => {
  const sockets: FakeSocket[] = [];
  const timers = new Map<number, { callback: () => void; delay: number }>();
  let timerID = 0;
  class FakeSocket {
    listeners: Record<string, ((event: any) => void)[]> = {};
    closed = false;
    constructor(public url: string) { sockets.push(this); }
    addEventListener(type: string, callback: (event: any) => void) {
      (this.listeners[type] ||= []).push(callback);
    }
    emit(type: string, data?: unknown) { this.listeners[type]?.forEach(fn => fn({ data: JSON.stringify(data) })); }
    close() { this.closed = true; this.emit('close'); }
  }
  Object.assign(globalThis, {
    WebSocket: FakeSocket,
    window: {
      location: { protocol: 'http:', host: 'localhost' },
      setTimeout: (callback: () => void, delay: number) => { timers.set(++timerID, { callback, delay }); return timerID; },
      clearTimeout: (id: number) => timers.delete(id)
    }
  });
  const states: GameState[] = [];
  const statuses: boolean[] = [];
  const stop = startGameConnection('room', state => states.push(state), status => statuses.push(status));
  const first = sockets[0];
  first.emit('open');
  first.emit('close');
  assert.equal([...timers.values()][0].delay, 500);
  const runTimer = () => { const [id, timer] = [...timers.entries()][0]; timers.delete(id); timer.callback(); };
  runTimer();
  first.emit('message', { type: 'state', state: { revision: 90 } });
  first.emit('open');
  assert.equal(states.length, 0);
  assert.equal(statuses.at(-1), false);
  for (let i = 0; i < 8; i++) { sockets.at(-1)!.emit('close'); runTimer(); }
  sockets.at(-1)!.emit('close');
  assert.equal([...timers.values()][0].delay, 10000);
  runTimer();
  sockets.at(-1)!.emit('open');
  sockets.at(-1)!.emit('message', { type: 'state', state: { revision: 2 } });
  assert.equal(states[0].revision, 2);
  stop();
  assert.equal(timers.size, 0);
  assert.equal(sockets.at(-1)!.closed, true);
  sockets.at(-1)!.emit('message', { type: 'state', state: { revision: 3 } });
  assert.equal(states.length, 1);
});
