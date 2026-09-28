import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const source = await readFile(path.join(root, 'client-portal', 'app.js'), 'utf8');
const start = source.indexOf('  function streamEventId(');
const end = source.indexOf('  function startGlobalStream()', start);
assert.ok(start >= 0 && end > start, 'stream renderer must be present');

const list = {
  children: [],
  querySelector: () => null,
  insertBefore(row, next) {
    const index = next ? this.children.indexOf(next) : -1;
    if (index < 0) this.children.push(row);
    else this.children.splice(index, 0, row);
  },
  get lastElementChild() { return this.children.at(-1); },
};
const context = {
  renderedGlobalEventIds: new Set(),
  isStreamPaused: false,
  document: { createElement: () => ({ dataset: {}, classList: { remove() {} }, remove() {} }) },
  $: (id) => id === 'logsList' ? list : null,
  escapeHtml: String,
  fmt: String,
  setTimeout: () => 0,
  clearTimeout() {},
};
vm.runInNewContext(`${source.slice(start, end)}\nthis.stream = { sortStreamEvents, renderStreamEvent, streamEventId };`, context);
const { sortStreamEvents, renderStreamEvent, streamEventId } = context.stream;

const event = (id, seconds) => ({ id, timestamp: `2026-09-28T12:${seconds}Z`, model: 'test-model', status: '200', totalTokens: 10 });
const snapshot = [event('older', '44:13'), event('newer', '46:40'), event('middle', '45:21')];
for (const item of sortStreamEvents(snapshot)) renderStreamEvent(item, false, 'append');
assert.deepEqual(list.children.map((row) => row.dataset.streamTime), [
  String(Date.parse(snapshot[1].timestamp)),
  String(Date.parse(snapshot[2].timestamp)),
  String(Date.parse(snapshot[0].timestamp)),
], 'initial snapshot must render newest first');

renderStreamEvent(event('late-private', '45:50'));
assert.equal(list.children[1].dataset.streamTime, String(Date.parse('2026-09-28T12:45:50Z')), 'late private event belongs at its timestamp');
renderStreamEvent(event('live-new', '47:00'));
assert.equal(list.children[0].dataset.streamTime, String(Date.parse('2026-09-28T12:47:00Z')), 'new live request must appear first');
renderStreamEvent(snapshot[1]);
assert.equal(list.children.length, 5, 'duplicate global event must not be rendered twice');
assert.equal(streamEventId({ requestId: 'request-1' }), 'request-1', 'private events use request ID when no event ID exists');
assert.equal(streamEventId({ id: 'request-1:completed', requestId: 'request-1' }), 'request-1', 'private completion and global request use the same ID');
renderStreamEvent({ id: 'request-1:started', requestId: 'request-1', type: 'request.started', timestamp: '2026-09-28T12:48:00Z' });
assert.equal(list.children.length, 5, 'in-flight lifecycle event is not a completed request row');
renderStreamEvent({ id: 'request-1:completed', requestId: 'request-1', type: 'request.completed', timestamp: '2026-09-28T12:48:00Z' });
renderStreamEvent({ id: 'request-1', timestamp: '2026-09-28T12:48:00Z' });
assert.equal(list.children.length, 6, 'private completion and global snapshot must deduplicate');

list.children.length = 0;
context.renderedGlobalEventIds.clear();
renderStreamEvent(event('private-first', '45:50'));
for (const item of sortStreamEvents(snapshot)) renderStreamEvent(item, false, 'append');
assert.deepEqual(list.children.map((row) => Number(row.dataset.streamTime)), [
  Date.parse(snapshot[1].timestamp),
  Date.parse('2026-09-28T12:45:50Z'),
  Date.parse(snapshot[2].timestamp),
  Date.parse(snapshot[0].timestamp),
], 'global snapshot arriving after private stream must still be newest first');

console.log('Client stream ordering passed: snapshot, out-of-order private, live, deduplication.');
