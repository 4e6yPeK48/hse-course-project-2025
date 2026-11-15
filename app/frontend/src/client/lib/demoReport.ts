import type { GameMetrics, GameReport } from '../types';

// A fixed teaching example, deliberately independent of live games and random coins.
const rows = [
  [1, 12, 3, 1, 0, 0, 0], [2, 9, 4, 2, 1, 0, 2],
  [3, 6, 5, 2, 3, 0, 2.5], [4, 4, 5, 2, 5, 4, 3],
  [5, 2, 5, 2, 7, 4, 3.2], [6, 1, 4, 2, 9, 4.5, 3.1],
  [7, 0, 3, 2, 11, 4.5, 3], [8, 0, 2, 1, 13, 4.3, 2.9],
  [9, 0, 1, 1, 14, 4.3, 2.8], [10, 0, 0, 0, 16, 4.2, 2.7]
];
const days = rows.map(([day, ready, in_progress, review, done, lead, cycle]) => ({
  day, iteration: day <= 5 ? 1 : 2, partial: day === 10,
  metrics: {
    cfd: { ready, in_progress, review, done }, wip: in_progress + review,
    lead_time: lead, avg_task_cycle_time: cycle, velocity: done / day,
    blocked: day === 3 || day === 4 ? 1 : 0, retro_days: day > 5 ? 1 : 0,
    total_blocked_days: day < 3 ? 0 : day === 3 ? 1 : 2,
    total_penalties: 0, last_retro_throughput: day > 5 ? 7 : 0
  } satisfies GameMetrics
}));
export const demoReport: GameReport = {
  code: 'DEMO', revision: 1, phase: 'finished', finished: true,
  summary: days[days.length - 1].metrics, days,
  iterations: [
    { number: 1, start_day: 1, end_day: 5, partial: false, throughput: 7, average_wip: 6.2, lead_time: 4, cycle_time: 3.2 },
    { number: 2, start_day: 6, end_day: 10, partial: true, throughput: 9, average_wip: 3.2, lead_time: 4.4, cycle_time: 2.3 }
  ]
};
