import { useEffect, useRef, useState } from 'react';
import type { GameState } from '../types';
import { startGameConnection } from './connection';

export const useGameSocket = (gameCode: string, onState: (state: GameState) => void) => {
  const onStateRef = useRef(onState);
  onStateRef.current = onState;
  const [connected, setConnected] = useState(false);
  useEffect(() => {
    if (!gameCode) return;
    return startGameConnection(gameCode, state => onStateRef.current(state), setConnected);
  }, [gameCode]);
  return { connected };
};
