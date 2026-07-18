import { describe, it, expect, beforeEach } from 'vitest';
import { prefillEventRange } from '../../cmd/pvs-ui/static/js/events.js';

function setupForm() {
  document.body.innerHTML = `
    <input type="datetime-local" id="event-start-at">
    <input type="datetime-local" id="event-end-at">
    <select id="event-type-select"></select>
  `;
}

describe('prefillEventRange', () => {
  beforeEach(setupForm);

  it('fills start and end datetime for a same-day selection', () => {
    const start = new Date(2026, 6, 17, 9, 0).getTime();
    const end   = new Date(2026, 6, 17, 15, 0).getTime();
    prefillEventRange(start, end);
    expect(document.getElementById('event-start-at').value).toBe('2026-07-17T09:00');
    expect(document.getElementById('event-end-at').value).toBe('2026-07-17T15:00');
  });

  it('fills start and end datetime for a multi-day selection', () => {
    const start = new Date(2026, 6, 17, 9, 0).getTime();
    const end   = new Date(2026, 6, 19, 15, 0).getTime();
    prefillEventRange(start, end);
    expect(document.getElementById('event-start-at').value).toBe('2026-07-17T09:00');
    expect(document.getElementById('event-end-at').value).toBe('2026-07-19T15:00');
  });

  it('does nothing if the form is not present', () => {
    document.body.innerHTML = '';
    expect(() => prefillEventRange(0, 0)).not.toThrow();
  });
});
