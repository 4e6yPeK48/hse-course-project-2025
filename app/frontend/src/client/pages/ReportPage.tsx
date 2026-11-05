import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { getJson } from '../lib/http';
import { demoReport } from '../lib/demoReport';
import type { GameReport } from '../types';

const stages = [
  { key: 'done', label: 'Done', color: '#4ec4b3' },
  { key: 'review', label: 'Review', color: '#c29cff' },
  { key: 'in_progress', label: 'In progress', color: '#f2b257' },
  { key: 'ready', label: 'Ready', color: '#7897cc' }
];
const number = (value: number) => value.toLocaleString('ru-RU', { maximumFractionDigits: 1 });

function FlowChart({ report }: { report: GameReport }) {
  const { days } = report;
  if (!days.length) return <p className="help">После первого игрового дня здесь появятся снимки потока.</p>;
  const max = Math.max(1, ...days.map(day => stages.reduce((sum, stage) => sum + (day.metrics.cfd[stage.key] || 0), 0)));
  const x = (index: number) => 54 + index * 690 / Math.max(1, days.length - 1);
  const y = (value: number) => 250 - value / max * 210;
  const bands = stages.map((stage, stageIndex) => {
    const bottom = days.map(day => stages.slice(0, stageIndex).reduce((sum, s) => sum + (day.metrics.cfd[s.key] || 0), 0));
    const top = bottom.map((value, index) => value + (days[index].metrics.cfd[stage.key] || 0));
    const points = days.length === 1
      ? `54,${y(top[0])} 744,${y(top[0])} 744,${y(bottom[0])} 54,${y(bottom[0])}`
      : [...top.map((value, index) => `${x(index)},${y(value)}`), ...bottom.map((value, index) => `${x(index)},${y(value)}`).reverse()].join(' ');
    return { ...stage, points };
  });
  return <figure className="flow-chart">
    <svg viewBox="0 0 780 290" role="img" aria-label="Накопительная диаграмма потока задач по игровым дням">
      {[0, 0.5, 1].map(tick => <g key={tick}><line x1="54" y1={y(max * tick)} x2="744" y2={y(max * tick)} stroke="#36415c" /><text x="43" y={y(max * tick) + 4} textAnchor="end">{number(max * tick)}</text></g>)}
      {bands.map(band => <polygon key={band.key} points={band.points} fill={band.color} fillOpacity="0.85" />)}
      {days.map((day, index) => <text key={day.day} x={x(index)} y="276" textAnchor="middle">{day.day}</text>)}
    </svg>
    <figcaption>По горизонтали — игровой день, по вертикали — количество задач. Каждая полоса показывает задачи в одной стадии; Done накапливается. Точные значения доступны в таблице ниже.</figcaption>
    <div className="inline chart-legend">{stages.map(stage => <span key={stage.key}><i style={{ background: stage.color }} />{stage.label}</span>)}</div>
  </figure>;
}

export function ReportView({ report, demo = false }: { report: GameReport; demo?: boolean }) {
  const metrics = report.summary;
  return <main className="page report-page"><div className="shell stack">
    <nav className="inline" aria-label="Навигация"><Link to="/">Featureban</Link><Link to="/start">Создать игру</Link>{!demo && <Link to={`/game/${encodeURIComponent(report.code)}`}>Вернуться к игре</Link>}</nav>
    <header className="card report-heading">
      <div className="badge">{demo ? 'Демонстрационный отчёт' : report.finished ? 'Игра завершена' : 'Игра продолжается'}</div>
      <h1>Поток команды · {report.code}</h1>
      <p className="subtitle">От дневной загрузки к разговору о том, что помогает завершать работу.</p>
      {demo ? <p className="demo-note">Это фиксированный учебный пример на реалистичных данных. Он не связан с живой игрой и не симулирует игроков или ботов.</p> : <a className="btn" href={`/api/game/${encodeURIComponent(report.code)}/report?format=csv`} download>Скачать CSV</a>}
    </header>
    <section className="report-summary" aria-label="Итоговые метрики">
      {[['WIP · задач в работе', number(metrics.wip)], ['Завершено задач', number(metrics.cfd.done || 0)], ['Lead time · дней на проект', number(metrics.lead_time)], ['Cycle time · дней на задачу', number(metrics.avg_task_cycle_time)]].map(([label, value]) => <div className="card" key={label}><div className="help">{label}</div><strong className="report-number">{value}</strong></div>)}
    </section>
    <section className="card"><h2>Как менялся поток</h2><FlowChart report={report} /></section>
    <section className="card"><h2>Сравнение итераций</h2><p className="help">Throughput — завершённые задачи за интервал. WIP усреднён по дневным снимкам. Lead time считается для завершённых проектов, cycle time — для завершённых задач. Нулевое время при отсутствии завершений не означает мгновенную работу.</p>
      <div className="table-scroll"><table><caption>Итерации и длительность работы</caption><thead><tr>{['Итерация', 'Дни', 'Throughput', 'Средний WIP', 'Lead time, дн.', 'Cycle time, дн.'].map(label => <th scope="col" key={label}>{label}</th>)}</tr></thead><tbody>{report.iterations.map(iteration => <tr key={iteration.number}><th scope="row">{iteration.number}{iteration.partial && <span className="partial-label">Неполная</span>}</th><td>{iteration.start_day}–{iteration.end_day}</td><td>{iteration.throughput}</td><td>{number(iteration.average_wip)}</td><td>{number(iteration.lead_time)}</td><td>{number(iteration.cycle_time)}</td></tr>)}</tbody></table></div>
      {!report.iterations.length && <p className="help">Пока нет завершённых игровых дней.</p>}
    </section>
    <section className="card"><h2>Дневные снимки</h2><p className="help">Неполный снимок — текущий незакрытый день или день досрочного завершения. Неполная итерация ещё не достигла следующего ретро.</p><div className="table-scroll"><table><caption>Данные CFD и метрик по дням</caption><thead><tr>{['День', 'Итерация', 'Ready', 'In progress', 'Review', 'Done', 'WIP', 'Lead time, дн.', 'Cycle time, дн.'].map(label => <th scope="col" key={label}>{label}</th>)}</tr></thead><tbody>{report.days.map(day => <tr key={day.day}><th scope="row">{day.day}{day.partial && <span className="partial-label">Неполный</span>}</th><td>{day.iteration}</td><td>{day.metrics.cfd.ready || 0}</td><td>{day.metrics.cfd.in_progress || 0}</td><td>{day.metrics.cfd.review || 0}</td><td>{day.metrics.cfd.done || 0}</td><td>{day.metrics.wip}</td><td>{number(day.metrics.lead_time)}</td><td>{number(day.metrics.avg_task_cycle_time)}</td></tr>)}</tbody></table></div></section>
  </div></main>;
}

export default function ReportPage({ demo = false }: { demo?: boolean }) {
  const { gamecode = '' } = useParams();
  const [report, setReport] = useState<GameReport | null>(demo ? demoReport : null);
  const [error, setError] = useState('');
  useEffect(() => {
    if (demo) { setReport(demoReport); return; }
    let active = true;
    setReport(null); setError('');
    getJson<GameReport>(`/api/game/${encodeURIComponent(gamecode)}/report`).then(data => { if (active) setReport(data); }).catch(error => { if (active) setError(error.message); });
    return () => { active = false; };
  }, [gamecode, demo]);
  if (report) return <ReportView report={report} demo={demo} />;
  return <main className="page"><div className="card stack"><h1>Отчёт по игре</h1><p role="status">{error || 'Загружаем отчёт…'}</p>{error && <p className="help">Отчёт доступен ведущему в браузере, где он создал игру.</p>}<Link to={`/game/${encodeURIComponent(gamecode)}`}>Вернуться к игре</Link></div></main>;
}
