import { test } from 'node:test';
import assert from 'node:assert/strict';
import { demoReport } from '../src/client/lib/demoReport';

test('bundled example has consistent completed work and iteration totals', () => {
  let completed = 0;
  for (const day of demoReport.days) {
    assert.ok(day.metrics.cfd.done >= completed);
    assert.equal(day.metrics.wip, day.metrics.cfd.in_progress + day.metrics.cfd.review);
    completed = day.metrics.cfd.done;
  }
  assert.equal(demoReport.summary.wip, 0);
  assert.equal(demoReport.summary.cfd.done, completed);
  assert.equal(demoReport.iterations.reduce((sum, iteration) => sum + iteration.throughput, 0), completed);
});
