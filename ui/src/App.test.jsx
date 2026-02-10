import { fireEvent, render, screen, waitFor } from '@testing-library/preact';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import App from './App.jsx';
import * as api from './api.js';

vi.mock('./api.js', () => ({
  addTaskLabels: vi.fn(),
  closeTasks: vi.fn(),
  getAuthMe: vi.fn(),
  getInfo: vi.fn(),
  getTask: vi.fn(),
  listTasks: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
  removeTaskLabels: vi.fn(),
  reopenTasks: vi.fn(),
  updateTask: vi.fn(),
}));

const TASKS = [
  {
    id: 'gr-aa11',
    title: 'First task',
    status: 'open',
    priority: 2,
    type: 'task',
    updated_at: '2026-02-09T12:00:00Z',
  },
  {
    id: 'gr-bb22',
    title: 'Second task',
    status: 'blocked',
    priority: 3,
    type: 'bug',
    updated_at: '2026-02-09T12:01:00Z',
  },
];

function seedDefaultAPIMocks() {
  api.getAuthMe.mockResolvedValue({ auth_required: false, authenticated: false });
  api.getInfo.mockResolvedValue({ project_prefix: 'gr', schema_version: 8 });
  api.listTasks.mockResolvedValue(TASKS);
  api.getTask.mockResolvedValue({
    id: 'gr-aa11',
    title: 'First task',
    status: 'open',
    type: 'task',
    priority: 2,
    labels: [],
    deps: [],
    created_at: '2026-02-09T12:00:00Z',
    updated_at: '2026-02-09T12:00:00Z',
  });
  api.updateTask.mockResolvedValue({});
  api.addTaskLabels.mockResolvedValue(['bulk']);
  api.removeTaskLabels.mockResolvedValue([]);
  api.closeTasks.mockResolvedValue({ ids: [] });
  api.reopenTasks.mockResolvedValue({ ids: [] });
  api.login.mockResolvedValue({ authenticated: true, auth_required: true, username: 'admin', auth_type: 'session' });
  api.logout.mockResolvedValue(null);
}

async function renderListApp() {
  window.location.hash = '#/';
  render(<App />);
  await screen.findByText('First task');
}

describe('App list behavior', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    seedDefaultAPIMocks();
  });

  it('opens shortcut help with ?', async () => {
    await renderListApp();

    fireEvent.keyDown(window, { key: '?', shiftKey: true });

    expect(screen.getByText('Keyboard shortcuts')).toBeTruthy();
  });

  it('closes shortcut help with Escape', async () => {
    await renderListApp();

    fireEvent.keyDown(window, { key: '?', shiftKey: true });
    expect(screen.getByText('Keyboard shortcuts')).toBeTruthy();

    fireEvent.keyDown(window, { key: 'Escape' });

    await waitFor(() => {
      expect(screen.queryByText('Keyboard shortcuts')).toBeNull();
    });
  });

  it('focuses search input with /', async () => {
    const user = userEvent.setup();
    await renderListApp();

    const input = screen.getByPlaceholderText('Search title/description/notes');
    document.body.focus();
    await user.keyboard('/');

    expect(document.activeElement).toBe(input);
  });

  it('toggles row selection via checkbox', async () => {
    const user = userEvent.setup();
    await renderListApp();

    const firstCheckbox = screen.getByLabelText('Select task gr-aa11');
    expect(firstCheckbox.checked).toBe(false);

    await user.click(firstCheckbox);

    await waitFor(() => {
      expect(screen.getByLabelText('Select task gr-aa11').checked).toBe(true);
    });
  });

  it('toggles visible selection via header checkbox', async () => {
    const user = userEvent.setup();
    await renderListApp();

    const headerCheckbox = screen.getByLabelText('Select all visible tasks');
    await user.click(headerCheckbox);

    await waitFor(() => {
      expect(screen.getByLabelText('Select task gr-aa11').checked).toBe(true);
      expect(screen.getByLabelText('Select task gr-bb22').checked).toBe(true);
    });

    await user.click(screen.getByLabelText('Select all visible tasks'));
    await waitFor(() => {
      expect(screen.getByLabelText('Select task gr-aa11').checked).toBe(false);
      expect(screen.getByLabelText('Select task gr-bb22').checked).toBe(false);
    });
  });

  it('navigates to detail when task title is clicked', async () => {
    const user = userEvent.setup();
    await renderListApp();

    await user.click(screen.getByRole('link', { name: 'First task' }));

    await waitFor(() => {
      expect(window.location.hash).toBe('#/tasks/gr-aa11');
    });
  });

  it('bulk close calls API with selected ids', async () => {
    const user = userEvent.setup();
    await renderListApp();

    await user.click(screen.getByLabelText('Select task gr-aa11'));
    await user.click(screen.getByLabelText('Select task gr-bb22'));
    await user.click(screen.getByRole('button', { name: 'Close selected' }));

    await waitFor(() => {
      expect(api.closeTasks).toHaveBeenCalledWith('gr', ['gr-aa11', 'gr-bb22']);
    });
  });

  it('bulk add labels reports partial failure', async () => {
    const user = userEvent.setup();
    api.addTaskLabels.mockImplementation((_project, id) => {
      if (id === 'gr-aa11') {
        return Promise.resolve(['bulk']);
      }
      return Promise.reject(new Error('boom'));
    });

    await renderListApp();

    await user.click(screen.getByLabelText('Select task gr-aa11'));
    await user.click(screen.getByLabelText('Select task gr-bb22'));
    await user.type(screen.getByPlaceholderText('labels for selected (comma-separated)'), 'bulk');
    await user.click(screen.getByRole('button', { name: 'Add labels' }));

    await waitFor(() => {
      expect(screen.getByText('Added label(s) to 1/2 task(s).')).toBeTruthy();
      expect(screen.getByText(/Failed on 1 task\(s\): boom/)).toBeTruthy();
    });
  });

  it('shows login form when auth is required and signs in', async () => {
    const user = userEvent.setup();
    const unauthorized = new Error('unauthorized');
    unauthorized.status = 401;

    api.getAuthMe
      .mockReset()
      .mockRejectedValueOnce(unauthorized)
      .mockResolvedValueOnce({ auth_required: true, authenticated: true, username: 'admin', auth_type: 'session' });

    window.location.hash = '#/';
    render(<App />);

    await screen.findByRole('heading', { name: 'grns sign in' });
    await user.type(screen.getByLabelText('Username'), 'admin');
    await user.type(screen.getByLabelText('Password'), 'password-123');
    await user.click(screen.getByRole('button', { name: 'Sign in' }));

    await waitFor(() => {
      expect(api.login).toHaveBeenCalledWith('admin', 'password-123');
    });

    await screen.findByText('First task');
  });

  it('sign out clears local bearer fallback token', async () => {
    const user = userEvent.setup();

    api.getAuthMe
      .mockReset()
      .mockResolvedValueOnce({ auth_required: true, authenticated: true, username: 'admin', auth_type: 'session' })
      .mockResolvedValue({ auth_required: false, authenticated: false });

    localStorage.setItem('grns_api_token', 'test-token');

    window.location.hash = '#/';
    render(<App />);

    await screen.findByText('First task');
    await user.click(screen.getByRole('button', { name: 'Sign out' }));

    await waitFor(() => {
      expect(api.logout).toHaveBeenCalled();
      expect(localStorage.getItem('grns_api_token')).toBeNull();
    });
  });
});
