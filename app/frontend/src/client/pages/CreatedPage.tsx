import React from 'react';
import { Link, useParams } from 'react-router-dom';

const CreatedPage: React.FC = () => {
  const { gamecode = '' } = useParams();
  const joinUrl = `${window.location.origin}/joining/${encodeURIComponent(gamecode)}`;

  return (
    <div className="page">
      <div className="shell" style={{ maxWidth: 520 }}>
        <div className="card">
          <div className="stack">
            <div>
              <h2 style={{ margin: 0, fontFamily: 'IBM Plex Serif, serif' }}>Игра успешно создана!</h2>
              <div className="help" style={{ marginTop: 6 }}>
                Код игры: <strong style={{ color: 'var(--accent)' }}>{gamecode}</strong>
              </div>
            </div>
            <div className="help">
              Игроки могут подключиться по ссылке: <a href={joinUrl}>{joinUrl}</a>
            </div>
            <Link className="btn" to={`/game/${encodeURIComponent(gamecode)}`}>
              Открыть панель ведущего
            </Link>
          </div>
        </div>
      </div>
    </div>
  );
};

export default CreatedPage;
