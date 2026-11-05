import { useEffect, useMemo, useRef, useState, type FC } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { getJson, postJson } from '../lib/http';
import { useGameSocket } from '../lib/useGameSocket';
import type { GameState, Team, Member } from '../types';

type JoinResponse = {
  redirect_to?: string;
  game_code?: string;
  player_id?: string;
  error?: string;
};

const JoiningPage: FC = () => {
  const { gamecode = '' } = useParams();
  const navigate = useNavigate();
  const [teams, setTeams] = useState<Team[]>([]);
  const [teamId, setTeamId] = useState('');
  const [nickname, setNickname] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const latestRevision = useRef(-1);
  const receiveState = (state: GameState) => {
    if (state.revision <= latestRevision.current) return;
    latestRevision.current = state.revision;
    const teamsList = state.teams;
    if (!teamsList) return;
    setTeams(teamsList);
    if (teamsList[0]) {
      setTeamId((prev) => prev || teamsList[0].id);
    }
  };
  useGameSocket(gamecode, receiveState);

  useEffect(() => {
    if (!gamecode) return;
    let active = true;
    latestRevision.current = -1;
    setTeams([]);
    setTeamId('');
    getJson<Member>(`/api/game/${encodeURIComponent(gamecode)}/me`).then(() => { if (active) navigate(`/game/${encodeURIComponent(gamecode)}`, { replace: true }); }).catch(() => {});
    getJson<GameState>(`/api/game/${encodeURIComponent(gamecode)}`)
      .then((data) => {
        if (!active) return;
        receiveState(data);
      })
      .catch(() => { if (active) setError('Сервер недоступен.'); });
    return () => { active = false; };
  }, [gamecode, navigate]);

  const options = useMemo(() => teams.map((team) => ({
    id: team.id,
    label: team.name
  })), [teams]);

  const join = async () => {
    if (!gamecode) {
      setError('Некорректная ссылка: отсутствует код игры.');
      return;
    }
    if (!teamId) {
      setError('Выберите команду.');
      return;
    }
    if (!nickname.trim()) {
      setError('Введите никнейм.');
      return;
    }

    setError('');
    setBusy(true);

    try {
      const data = await postJson<JoinResponse>('/api/join', {
        game_code: gamecode,
        nickname: nickname.trim(),
        team_id: teamId
      });

      navigate(`/game/${encodeURIComponent(data.game_code || gamecode)}`);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Сервер недоступен. Попробуйте снова.';
      setError(message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="page">
      <div className="shell" style={{ maxWidth: 420 }}>
        <div className="card">
          <div className="stack">
            <div>
              <h2 style={{ margin: 0, fontFamily: 'IBM Plex Serif, serif' }}>Подключение к игре</h2>
              <div className="help">Код игры: <strong>{gamecode || 'неизвестно'}</strong></div>
            </div>

            <label className="help" htmlFor="team">Команда</label>
            <select
              id="team"
              className="field"
              value={teamId}
              onChange={(event) => setTeamId(event.target.value)}
            >
              {options.length === 0 ? (
                <option value="">Загрузка команд...</option>
              ) : (
                options.map((team) => (
                  <option key={team.id} value={team.id}>
                    {team.label}
                  </option>
                ))
              )}
            </select>

            <label className="help" htmlFor="nickname">Ваш никнейм</label>
            <input
              id="nickname"
              className="field"
              type="text"
              autoComplete="nickname"
              maxLength={20}
              placeholder="Введите никнейм..."
              value={nickname}
              onChange={(event) => setNickname(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  event.preventDefault();
                  join();
                }
              }}
            />

            <button className="btn" type="button" disabled={busy} onClick={join}>
              Войти в игру
            </button>
            <div className="error">{error}</div>
          </div>
        </div>
      </div>
    </div>
  );
};

export default JoiningPage;
