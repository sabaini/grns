import {
  buildHash,
  formatTime,
  isEditableTarget,
  parseHashRoute,
  parseListState,
  taskIDFromPath,
} from './App.jsx';

describe('App utility functions', () => {
  it('parses hash route and query params', () => {
    const parsed = parseHashRoute('#/tasks/gr-ab12?project=xy&limit=20');
    expect(parsed.path).toBe('/tasks/gr-ab12');
    expect(parsed.params.get('project')).toBe('xy');
    expect(parsed.params.get('limit')).toBe('20');
  });

  it('builds hash route with query params', () => {
    const params = new URLSearchParams({ project: 'gr', status: 'open' });
    expect(buildHash('/tasks/gr-ab12', params)).toBe('#/tasks/gr-ab12?project=gr&status=open');
  });

  it('parses list state with defaults and normalization', () => {
    const params = new URLSearchParams('status=open,invalid,blocked&limit=0&offset=-2&search=  hello  ');
    const state = parseListState(params);

    expect(state.search).toBe('hello');
    expect(state.statuses).toEqual(['open', 'blocked']);
    expect(state.limit).toBe(50);
    expect(state.offset).toBe(0);
  });

  it('extracts task id from detail path', () => {
    expect(taskIDFromPath('/tasks/gr-aa11')).toBe('gr-aa11');
    expect(taskIDFromPath('/')).toBe('');
  });

  it('formats timestamps and detects editable targets', () => {
    expect(formatTime('2026-02-09T12:34:56Z')).toBe('2026-02-09T12:34:56Z');
    expect(formatTime('')).toBe('—');

    const input = document.createElement('input');
    const div = document.createElement('div');

    expect(isEditableTarget(input)).toBe(true);
    expect(isEditableTarget(div)).toBe(false);
  });
});
