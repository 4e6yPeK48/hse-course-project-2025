import type { GameState } from '../types';

export function newerState(current: GameState | null, incoming: GameState): GameState {
  return current && incoming.revision <= current.revision ? current : incoming;
}

export function startGameConnection(code: string, onState: (state: GameState) => void, onStatus: (connected: boolean) => void) {
  let socket: WebSocket;
  let retryTimer: number | undefined;
  let attempt = 0;
  let stopped = false;
  const connect = () => {
    const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws';
    const current = new WebSocket(`${protocol}://${window.location.host}/ws/game?code=${encodeURIComponent(code)}`);
    socket = current;
    const active = () => !stopped && socket === current;
    current.addEventListener('open', () => {
      if (!active()) return;
      attempt = 0;
      onStatus(true);
    });
    current.addEventListener('message', event => {
      if (!active()) return;
      try {
        const payload = JSON.parse(event.data);
        if (payload.type === 'state' && payload.state) onState(payload.state);
      } catch { /* Ignore malformed messages; polling remains available. */ }
    });
    current.addEventListener('error', () => { if (active()) { onStatus(false); current.close(); } });
    current.addEventListener('close', () => {
      if (!active()) return;
      onStatus(false);
      retryTimer = window.setTimeout(connect, Math.min(500 * 2 ** attempt++, 10000));
    });
  };
  onStatus(false);
  connect();
  return () => {
    stopped = true;
    window.clearTimeout(retryTimer);
    socket.close();
  };
}
