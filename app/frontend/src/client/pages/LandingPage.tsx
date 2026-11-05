import React from 'react';

const LandingPage: React.FC = () => {
  return (
    <div className="page">
      <div className="shell">
        <div className="badge">
          <span className="dot" /> Командный симулятор продуктовой разработки
        </div>
        <div className="hero">
          <div>
            <h1 className="title">Featureban: меньше начать, больше завершить</h1>
            <p className="subtitle">
              Командная игра о потоке работы: берите задачи, справляйтесь с блокировками
              и меняйте WIP-лимиты на ретро. Сравните итерации по реальным метрикам вашей игры.
            </p>
            <div className="inline">
              <a className="btn" href="/start">Создать или войти</a>
              <a className="btn secondary" href="/demo">Посмотреть демо-отчёт</a>
            </div>
          </div>
          <div className="card">
            <h3 style={{ marginTop: 0 }}>Что внутри</h3>
            <p className="help" style={{ margin: 0 }}>
              Ведущий создаёт комнату и делится ссылкой. Игроки входят с уникальными именами,
              выбирают команды и получают собственные монетки на каждый игровой день.
            </p>
          </div>
        </div>

        <div style={{ marginTop: 24 }} className="layout-grid">
          <div className="card compact">
            <h4 style={{ marginTop: 0 }}>Роли</h4>
            <p className="help">Ведущий задает контекст, команды выбирают, что делать дальше.</p>
          </div>
          <div className="card compact">
            <h4 style={{ marginTop: 0 }}>Цель</h4>
            <p className="help">Завершить проекты: задачи проходят Ready → In progress → Review → Done, затем проект интегрируется.</p>
          </div>
          <div className="card compact">
            <h4 style={{ marginTop: 0 }}>Формат</h4>
            <p className="help">Орёл: заблокировать свою задачу и взять новую. Решка: взять, продвинуть или разблокировать задачу. На ретро обсудите поток и WIP.</p>
          </div>
        </div>

        <div style={{ marginTop: 18 }} className="help">
          Если у вас уже есть код комнаты, нажмите «Начать игру» и введите его.
        </div>
      </div>
    </div>
  );
};

export default LandingPage;
