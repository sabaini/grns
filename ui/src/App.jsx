import { useEffect, useMemo, useRef, useState } from 'preact/hooks';

import {
  addTaskLabels,
  closeTasks,
  getAuthMe,
  getInfo,
  getTask,
  listTasks,
  login,
  logout,
  removeTaskLabels,
  reopenTasks,
  updateTask,
} from './api.js';

const STATUS_OPTIONS = ['open', 'in_progress', 'blocked', 'deferred', 'pinned', 'closed', 'tombstone'];
const TASK_TYPE_OPTIONS = ['bug', 'feature', 'task', 'epic', 'chore'];
const PRIORITY_OPTIONS = [0, 1, 2, 3, 4];
const DEFAULT_STATUSES = ['open', 'in_progress', 'blocked', 'deferred', 'pinned'];
const PAGE_SIZE_OPTIONS = [20, 50, 100];
const DEFAULT_LIMIT = 50;

export function parseHashRoute(rawHash) {
  let hash = rawHash || '#/';
  if (hash.startsWith('#')) {
    hash = hash.slice(1);
  }
  if (!hash.startsWith('/')) {
    hash = `/${hash}`;
  }

  const [path, query = ''] = hash.split('?');
  return {
    path: path || '/',
    params: new URLSearchParams(query),
  };
}

export function buildHash(path, params) {
  const query = params.toString();
  return `#${path}${query ? `?${query}` : ''}`;
}

export function normalizeStatuses(values) {
  const set = new Set(values);
  return STATUS_OPTIONS.filter((status) => set.has(status));
}

export function parseListState(params) {
  const search = (params.get('search') || '').trim();

  const rawStatuses = (params.get('status') || '')
    .split(',')
    .map((value) => value.trim())
    .filter(Boolean);
  const statuses = normalizeStatuses(rawStatuses);

  const limitRaw = Number.parseInt(params.get('limit') || '', 10);
  const limit = Number.isFinite(limitRaw) && limitRaw > 0 ? limitRaw : DEFAULT_LIMIT;

  const offsetRaw = Number.parseInt(params.get('offset') || '', 10);
  const offset = Number.isFinite(offsetRaw) && offsetRaw >= 0 ? offsetRaw : 0;

  return {
    search,
    statuses: statuses.length > 0 ? statuses : DEFAULT_STATUSES,
    limit,
    offset,
  };
}

export function taskIDFromPath(path) {
  if (!path.startsWith('/tasks/')) {
    return '';
  }
  const id = path.slice('/tasks/'.length).split('/')[0].trim();
  return id;
}

function useHashRoute() {
  const [route, setRoute] = useState(window.location.hash || '#/');

  useEffect(() => {
    const onHashChange = () => setRoute(window.location.hash || '#/');
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

  return route;
}

function useDebouncedValue(value, delayMs) {
  const [debounced, setDebounced] = useState(value);

  useEffect(() => {
    const handle = window.setTimeout(() => setDebounced(value), delayMs);
    return () => window.clearTimeout(handle);
  }, [value, delayMs]);

  return debounced;
}

export function formatTime(value) {
  if (!value) {
    return '—';
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toISOString().slice(0, 19) + 'Z';
}

export function isSameArray(a, b) {
  if (a.length !== b.length) {
    return false;
  }
  for (let i = 0; i < a.length; i += 1) {
    if (a[i] !== b[i]) {
      return false;
    }
  }
  return true;
}

export function isEditableTarget(target) {
  if (!(target instanceof Element)) {
    return false;
  }
  if (target.closest('input, textarea, select, button, [contenteditable="true"]')) {
    return true;
  }
  return false;
}

export default function App() {
  const hash = useHashRoute();
  const route = useMemo(() => parseHashRoute(hash), [hash]);
  const listState = useMemo(() => parseListState(route.params), [route]);
  const debouncedSearch = useDebouncedValue(listState.search, 300);

  const [info, setInfo] = useState(null);
  const [tasks, setTasks] = useState([]);
  const [taskDetail, setTaskDetail] = useState(null);

  const [authChecked, setAuthChecked] = useState(false);
  const [authRequired, setAuthRequired] = useState(false);
  const [authMe, setAuthMe] = useState(null);
  const [authRefreshNonce, setAuthRefreshNonce] = useState(0);

  const [loadingInfo, setLoadingInfo] = useState(true);
  const [loadingTasks, setLoadingTasks] = useState(false);
  const [loadingDetail, setLoadingDetail] = useState(false);

  const [infoError, setInfoError] = useState('');
  const [listError, setListError] = useState('');
  const [detailError, setDetailError] = useState('');
  const [detailSaveError, setDetailSaveError] = useState('');
  const [detailSavingField, setDetailSavingField] = useState('');
  const [assigneeDraft, setAssigneeDraft] = useState('');
  const [labelDraft, setLabelDraft] = useState('');
  const [editingTextField, setEditingTextField] = useState('');
  const [textDraft, setTextDraft] = useState('');

  const [selectedTaskIDs, setSelectedTaskIDs] = useState([]);
  const [bulkLabelDraft, setBulkLabelDraft] = useState('');
  const [listActionError, setListActionError] = useState('');
  const [listActionInfo, setListActionInfo] = useState('');
  const [listActionInFlight, setListActionInFlight] = useState(false);
  const [activeTaskIndex, setActiveTaskIndex] = useState(0);
  const [showShortcutHelp, setShowShortcutHelp] = useState(false);

  const [loginUsername, setLoginUsername] = useState('');
  const [loginPassword, setLoginPassword] = useState('');
  const [loginError, setLoginError] = useState('');
  const [loggingIn, setLoggingIn] = useState(false);

  const searchInputRef = useRef(null);

  const projectFromHash = (route.params.get('project') || '').trim();
  const project = projectFromHash || info?.project_prefix || '';

  const isListRoute = route.path === '/';
  const detailID = taskIDFromPath(route.path);
  const isDetailRoute = detailID !== '';
  const canAccessAPI = !authRequired || Boolean(authMe?.authenticated);

  useEffect(() => {
    let alive = true;

    async function loadAuthState() {
      setAuthChecked(false);
      setLoginError('');
      try {
        const me = await getAuthMe();
        if (!alive) {
          return;
        }
        const required = Boolean(me?.auth_required);
        const authenticated = Boolean(me?.authenticated);
        setAuthRequired(required);
        setAuthMe(authenticated ? me : null);
      } catch (err) {
        if (!alive) {
          return;
        }
        if (err?.status === 401) {
          setAuthRequired(true);
          setAuthMe(null);
        } else {
          setInfoError(err.message || 'Failed to load auth state');
          setAuthRequired(false);
          setAuthMe(null);
        }
      } finally {
        if (alive) {
          setAuthChecked(true);
        }
      }
    }

    loadAuthState();
    return () => {
      alive = false;
    };
  }, [authRefreshNonce]);

  useEffect(() => {
    let alive = true;

    async function loadInfo() {
      if (!authChecked) {
        return;
      }
      if (authRequired && !authMe?.authenticated) {
        setLoadingInfo(false);
        setInfo(null);
        return;
      }

      setLoadingInfo(true);
      setInfoError('');
      try {
        const meta = await getInfo();
        if (!alive) {
          return;
        }
        setInfo(meta);
      } catch (err) {
        if (!alive) {
          return;
        }
        setInfoError(err.message || 'Failed to load server info');
      } finally {
        if (alive) {
          setLoadingInfo(false);
        }
      }
    }

    loadInfo();
    return () => {
      alive = false;
    };
  }, [authChecked, authRequired, authMe?.authenticated]);

  function currentListParams() {
    const params = {
      limit: String(listState.limit),
      offset: String(listState.offset),
      status: listState.statuses.join(','),
    };
    if (debouncedSearch) {
      params.search = debouncedSearch;
    }
    return params;
  }

  async function fetchListData() {
    if (!isListRoute || !project) {
      return false;
    }

    setLoadingTasks(true);
    setListError('');
    try {
      const rows = await listTasks(project, currentListParams());
      setTasks(rows);
      return true;
    } catch (err) {
      setListError(err.message || 'Failed to load tasks');
      return false;
    } finally {
      setLoadingTasks(false);
    }
  }

  async function onLoginSubmit(event) {
    event.preventDefault();
    const username = loginUsername.trim();
    if (!username || !loginPassword) {
      setLoginError('Username and password are required.');
      return;
    }

    setLoggingIn(true);
    setLoginError('');
    try {
      await login(username, loginPassword);
      setLoginPassword('');
      setAuthRefreshNonce((value) => value + 1);
    } catch (err) {
      setLoginError(err.message || 'Login failed');
    } finally {
      setLoggingIn(false);
    }
  }

  async function onLogoutClick() {
    try {
      await logout();
    } catch (_) {
      // keep UI flow simple even if server-side logout fails
    }
    setTasks([]);
    setTaskDetail(null);
    setAuthRefreshNonce((value) => value + 1);
  }

  useEffect(() => {
    if (!canAccessAPI || !isListRoute || !project) {
      return undefined;
    }

    let alive = true;
    setLoadingTasks(true);
    setListError('');

    listTasks(project, currentListParams())
      .then((rows) => {
        if (!alive) {
          return;
        }
        setTasks(rows);
      })
      .catch((err) => {
        if (!alive) {
          return;
        }
        setListError(err.message || 'Failed to load tasks');
      })
      .finally(() => {
        if (alive) {
          setLoadingTasks(false);
        }
      });

    return () => {
      alive = false;
    };
  }, [canAccessAPI, project, isListRoute, listState.limit, listState.offset, listState.statuses, debouncedSearch]);

  useEffect(() => {
    let alive = true;

    async function loadTaskDetail() {
      if (!canAccessAPI || !isDetailRoute || !project) {
        setTaskDetail(null);
        return;
      }
      setLoadingDetail(true);
      setDetailError('');
      setDetailSaveError('');
      try {
        const detail = await getTask(project, detailID);
        if (!alive) {
          return;
        }
        setTaskDetail(detail);
      } catch (err) {
        if (!alive) {
          return;
        }
        setTaskDetail(null);
        setDetailError(err.message || 'Failed to load task');
      } finally {
        if (alive) {
          setLoadingDetail(false);
        }
      }
    }

    loadTaskDetail();
    return () => {
      alive = false;
    };
  }, [canAccessAPI, project, isDetailRoute, detailID]);

  useEffect(() => {
    setSelectedTaskIDs((prev) => {
      const visible = new Set(tasks.map((task) => task.id));
      return prev.filter((id) => visible.has(id));
    });
    setActiveTaskIndex((prev) => {
      if (tasks.length === 0) {
        return 0;
      }
      return Math.min(prev, tasks.length - 1);
    });
  }, [tasks]);

  useEffect(() => {
    setAssigneeDraft(taskDetail?.assignee || '');
  }, [taskDetail?.id, taskDetail?.assignee]);

  useEffect(() => {
    setLabelDraft('');
  }, [taskDetail?.id]);

  useEffect(() => {
    setEditingTextField('');
    setTextDraft('');
  }, [taskDetail?.id]);

  useEffect(() => {
    if (!isListRoute) {
      setShowShortcutHelp(false);
    }
  }, [isListRoute]);

  function updateListHash(next) {
    if (!isListRoute) {
      return;
    }

    const params = new URLSearchParams(route.params);

    const search = (next.search || '').trim();
    if (search) {
      params.set('search', search);
    } else {
      params.delete('search');
    }

    const statuses = normalizeStatuses(next.statuses);
    if (statuses.length > 0 && !isSameArray(statuses, DEFAULT_STATUSES)) {
      params.set('status', statuses.join(','));
    } else {
      params.delete('status');
    }

    if (next.limit > 0 && next.limit !== DEFAULT_LIMIT) {
      params.set('limit', String(next.limit));
    } else {
      params.delete('limit');
    }

    if (next.offset > 0) {
      params.set('offset', String(next.offset));
    } else {
      params.delete('offset');
    }

    const nextHash = buildHash('/', params);
    if (window.location.hash !== nextHash) {
      window.location.hash = nextHash;
    }
  }

  function toggleStatus(status) {
    const selected = listState.statuses.includes(status)
      ? listState.statuses.filter((value) => value !== status)
      : [...listState.statuses, status];

    const normalized = normalizeStatuses(selected);
    updateListHash({
      ...listState,
      statuses: normalized.length > 0 ? normalized : STATUS_OPTIONS,
      offset: 0,
    });
  }

  function onSearchInput(event) {
    updateListHash({
      ...listState,
      search: event.currentTarget.value,
      offset: 0,
    });
  }

  function onLimitChange(event) {
    const nextLimit = Number.parseInt(event.currentTarget.value, 10);
    if (!Number.isFinite(nextLimit) || nextLimit <= 0) {
      return;
    }
    updateListHash({
      ...listState,
      limit: nextLimit,
      offset: 0,
    });
  }

  function goPrevPage() {
    updateListHash({
      ...listState,
      offset: Math.max(0, listState.offset - listState.limit),
    });
  }

  function goNextPage() {
    updateListHash({
      ...listState,
      offset: listState.offset + listState.limit,
    });
  }

  function openActiveTask() {
    if (tasks.length === 0) {
      return;
    }
    const index = Math.min(activeTaskIndex, tasks.length - 1);
    const id = tasks[index]?.id;
    if (!id) {
      return;
    }
    const nextHash = buildHash(`/tasks/${id}`, route.params);
    if (window.location.hash !== nextHash) {
      window.location.hash = nextHash;
    }
  }

  function toggleTaskSelection(id) {
    setSelectedTaskIDs((prev) => {
      const set = new Set(prev);
      if (set.has(id)) {
        set.delete(id);
      } else {
        set.add(id);
      }
      return Array.from(set);
    });
  }

  function toggleSelectAllVisible() {
    const visibleIDs = tasks.map((task) => task.id);
    if (visibleIDs.length === 0) {
      return;
    }
    const selectedSet = new Set(selectedTaskIDs);
    const allSelected = visibleIDs.every((id) => selectedSet.has(id));

    setSelectedTaskIDs((prev) => {
      const next = new Set(prev);
      if (allSelected) {
        visibleIDs.forEach((id) => next.delete(id));
      } else {
        visibleIDs.forEach((id) => next.add(id));
      }
      return Array.from(next);
    });
  }

  function clearSelection() {
    setSelectedTaskIDs([]);
  }

  async function onBulkClose() {
    if (!project || selectedTaskIDs.length === 0) {
      return;
    }
    const ids = [...selectedTaskIDs];

    setListActionInFlight(true);
    setListActionError('');
    setListActionInfo('');
    try {
      await closeTasks(project, ids);
      const refreshed = await fetchListData();
      setSelectedTaskIDs([]);
      setListActionInfo(refreshed ? `Closed ${ids.length} task(s).` : `Closed ${ids.length} task(s), but refresh failed.`);
    } catch (err) {
      setListActionError(err.message || 'Failed to close selected tasks');
    } finally {
      setListActionInFlight(false);
    }
  }

  async function onBulkReopen() {
    if (!project || selectedTaskIDs.length === 0) {
      return;
    }
    const ids = [...selectedTaskIDs];

    setListActionInFlight(true);
    setListActionError('');
    setListActionInfo('');
    try {
      await reopenTasks(project, ids);
      const refreshed = await fetchListData();
      setSelectedTaskIDs([]);
      setListActionInfo(refreshed ? `Reopened ${ids.length} task(s).` : `Reopened ${ids.length} task(s), but refresh failed.`);
    } catch (err) {
      setListActionError(err.message || 'Failed to reopen selected tasks');
    } finally {
      setListActionInFlight(false);
    }
  }

  async function onBulkAddLabels() {
    if (!project || selectedTaskIDs.length === 0) {
      return;
    }

    const labels = parseLabelDraft(bulkLabelDraft);
    if (labels.length === 0) {
      return;
    }

    const ids = [...selectedTaskIDs];

    setListActionInFlight(true);
    setListActionError('');
    setListActionInfo('');

    try {
      const results = await Promise.allSettled(ids.map((id) => addTaskLabels(project, id, labels)));
      const succeeded = results.filter((result) => result.status === 'fulfilled').length;
      const failed = results.length - succeeded;

      if (succeeded > 0) {
        const refreshed = await fetchListData();
        setBulkLabelDraft('');
        setListActionInfo(
          refreshed
            ? `Added label(s) to ${succeeded}/${ids.length} task(s).`
            : `Added label(s) to ${succeeded}/${ids.length} task(s), but refresh failed.`,
        );
      } else {
        setListActionInfo(`Added label(s) to ${succeeded}/${ids.length} task(s).`);
      }
      if (failed > 0) {
        const firstFailure = results.find((result) => result.status === 'rejected');
        const reason = firstFailure && firstFailure.reason ? String(firstFailure.reason.message || firstFailure.reason) : 'unknown error';
        setListActionError(`Failed on ${failed} task(s): ${reason}`);
      }
    } catch (err) {
      setListActionError(err.message || 'Failed to add labels to selected tasks');
    } finally {
      setListActionInFlight(false);
    }
  }

  async function runDetailAction(field, action, fallbackMessage) {
    setDetailSaveError('');
    setDetailSavingField(field);
    try {
      return await action();
    } catch (err) {
      setDetailSaveError(err.message || fallbackMessage || `Failed to update ${field}`);
      return null;
    } finally {
      setDetailSavingField('');
    }
  }

  async function reloadTaskDetail() {
    if (!project || !detailID) {
      return null;
    }
    const detail = await getTask(project, detailID);
    setTaskDetail(detail);
    return detail;
  }

  async function saveTaskPatch(field, patch) {
    if (!project || !taskDetail || !taskDetail.id) {
      return false;
    }

    const updated = await runDetailAction(
      field,
      () => updateTask(project, taskDetail.id, patch),
      `Failed to update ${field}`,
    );
    if (!updated) {
      return false;
    }
    setTaskDetail(updated);
    return true;
  }

  function onStatusChange(event) {
    const value = event.currentTarget.value;
    if (!taskDetail || value === taskDetail.status) {
      return;
    }
    saveTaskPatch('status', { status: value });
  }

  function onTypeChange(event) {
    const value = event.currentTarget.value;
    if (!taskDetail || value === taskDetail.type) {
      return;
    }
    saveTaskPatch('type', { type: value });
  }

  function onPriorityChange(event) {
    const value = Number.parseInt(event.currentTarget.value, 10);
    if (!taskDetail || !Number.isFinite(value) || value === taskDetail.priority) {
      return;
    }
    saveTaskPatch('priority', { priority: value });
  }

  function onAssigneeSave() {
    if (!taskDetail) {
      return;
    }
    const normalized = assigneeDraft.trim();
    const current = (taskDetail.assignee || '').trim();
    if (normalized === current) {
      return;
    }
    saveTaskPatch('assignee', { assignee: normalized });
  }

  function parseLabelDraft(value) {
    const labels = value
      .split(',')
      .map((label) => label.trim().toLowerCase())
      .filter(Boolean);
    return Array.from(new Set(labels));
  }

  async function onAddLabels() {
    if (!taskDetail || !taskDetail.id) {
      return;
    }
    const labels = parseLabelDraft(labelDraft);
    if (labels.length === 0) {
      return;
    }

    const updatedLabels = await runDetailAction(
      'labels',
      () => addTaskLabels(project, taskDetail.id, labels),
      'Failed to add labels',
    );
    if (!updatedLabels) {
      return;
    }

    setTaskDetail((prev) => {
      if (!prev) {
        return prev;
      }
      return { ...prev, labels: updatedLabels };
    });
    setLabelDraft('');
  }

  async function onRemoveLabel(label) {
    if (!taskDetail || !taskDetail.id) {
      return;
    }

    const updatedLabels = await runDetailAction(
      'labels',
      () => removeTaskLabels(project, taskDetail.id, [label]),
      'Failed to remove label',
    );
    if (!updatedLabels) {
      return;
    }

    setTaskDetail((prev) => {
      if (!prev) {
        return prev;
      }
      return { ...prev, labels: updatedLabels };
    });
  }

  async function onCloseTask() {
    if (!taskDetail || !taskDetail.id) {
      return;
    }
    const ok = await runDetailAction('close', () => closeTasks(project, [taskDetail.id]), 'Failed to close task');
    if (ok) {
      await reloadTaskDetail();
    }
  }

  async function onReopenTask() {
    if (!taskDetail || !taskDetail.id) {
      return;
    }
    const ok = await runDetailAction('reopen', () => reopenTasks(project, [taskDetail.id]), 'Failed to reopen task');
    if (ok) {
      await reloadTaskDetail();
    }
  }

  function onTombstoneTask() {
    if (!taskDetail) {
      return;
    }
    if (taskDetail.status === 'tombstone') {
      return;
    }
    saveTaskPatch('status', { status: 'tombstone' });
  }

  function startTextEdit(field) {
    if (!taskDetail) {
      return;
    }
    setDetailSaveError('');
    setEditingTextField(field);
    setTextDraft(taskDetail[field] || '');
  }

  function cancelTextEdit() {
    setEditingTextField('');
    setTextDraft('');
  }

  async function saveTextEdit(field) {
    if (!taskDetail) {
      return;
    }
    const nextValue = textDraft;
    const currentValue = taskDetail[field] || '';
    if (nextValue === currentValue) {
      cancelTextEdit();
      return;
    }
    const ok = await saveTaskPatch(field, { [field]: nextValue });
    if (ok) {
      cancelTextEdit();
    }
  }

  function renderEditableTextSection(label, field) {
    if (!taskDetail) {
      return null;
    }

    const isEditing = editingTextField === field;
    const value = taskDetail[field] || '';

    return (
      <div className="detail-section">
        <div className="detail-section-header">
          <h3>{label}</h3>
          {!isEditing ? (
            <button
              type="button"
              className="detail-edit-btn"
              onClick={() => startTextEdit(field)}
              disabled={detailSavingField !== ''}
            >
              Edit
            </button>
          ) : (
            <div className="inline-actions">
              <button type="button" onClick={() => saveTextEdit(field)} disabled={detailSavingField !== ''}>
                Save
              </button>
              <button type="button" onClick={cancelTextEdit} disabled={detailSavingField !== ''}>
                Cancel
              </button>
            </div>
          )}
        </div>

        {isEditing ? (
          <textarea
            className="detail-textarea"
            value={textDraft}
            onInput={(event) => setTextDraft(event.currentTarget.value)}
            rows={6}
            disabled={detailSavingField !== ''}
          />
        ) : (
          <pre>{value || '—'}</pre>
        )}
      </div>
    );
  }

  useEffect(() => {
    if (!isListRoute) {
      return undefined;
    }

    function onKeyDown(event) {
      if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey) {
        return;
      }

      if (event.key === 'Escape' && showShortcutHelp) {
        event.preventDefault();
        setShowShortcutHelp(false);
        return;
      }

      if (isEditableTarget(event.target)) {
        return;
      }

      if (event.key === '?' || (event.shiftKey && event.key === '/')) {
        event.preventDefault();
        setShowShortcutHelp((prev) => !prev);
        return;
      }

      if (event.key === '/' && !event.shiftKey) {
        event.preventDefault();
        if (searchInputRef.current) {
          searchInputRef.current.focus();
          searchInputRef.current.select();
        }
        return;
      }

      if (showShortcutHelp) {
        return;
      }

      if (loadingTasks || listActionInFlight) {
        return;
      }

      if (event.shiftKey && event.key.toLowerCase() === 'a') {
        event.preventDefault();
        toggleSelectAllVisible();
        return;
      }

      if (!event.shiftKey && (event.key === 'x' || event.key === 'X')) {
        event.preventDefault();
        if (tasks.length === 0) {
          return;
        }
        const index = Math.min(activeTaskIndex, tasks.length - 1);
        const id = tasks[index]?.id || tasks[0]?.id;
        if (id) {
          toggleTaskSelection(id);
        }
        return;
      }

      if (event.key === 'Enter') {
        event.preventDefault();
        openActiveTask();
        return;
      }

      if (event.key === 'ArrowDown' || event.key === 'j') {
        event.preventDefault();
        if (tasks.length === 0) {
          return;
        }
        setActiveTaskIndex((prev) => Math.min(prev + 1, tasks.length - 1));
        return;
      }

      if (event.key === 'ArrowUp' || event.key === 'k') {
        event.preventDefault();
        if (tasks.length === 0) {
          return;
        }
        setActiveTaskIndex((prev) => Math.max(prev - 1, 0));
      }
    }

    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [isListRoute, tasks, activeTaskIndex, loadingTasks, listActionInFlight, showShortcutHelp, route.params]);

  const page = Math.floor(listState.offset / listState.limit) + 1;
  const canGoPrev = listState.offset > 0;
  const canGoNext = tasks.length === listState.limit;

  const selectedSet = new Set(selectedTaskIDs);
  const selectedCount = selectedTaskIDs.length;
  const allVisibleSelected = tasks.length > 0 && tasks.every((task) => selectedSet.has(task.id));
  const selectedByStatus = STATUS_OPTIONS
    .map((status) => ({
      status,
      count: tasks.reduce((total, task) => {
        if (!selectedSet.has(task.id)) {
          return total;
        }
        return task.status === status ? total + 1 : total;
      }, 0),
    }))
    .filter((entry) => entry.count > 0);

  if (!authChecked) {
    return (
      <main className="container">
        <h1>grns web ui</h1>
        <div className="meta">Checking authentication…</div>
      </main>
    );
  }

  if (authRequired && !authMe?.authenticated) {
    return (
      <main className="container">
        <header className="header">
          <h1>grns sign in</h1>
          <div className="meta">Sign in with a provisioned admin user.</div>
        </header>

        <form className="detail-card" onSubmit={onLoginSubmit}>
          <div className="detail-section">
            <label>
              Username
              <input
                type="text"
                autoComplete="username"
                value={loginUsername}
                onInput={(event) => setLoginUsername(event.currentTarget.value)}
                disabled={loggingIn}
              />
            </label>
          </div>
          <div className="detail-section">
            <label>
              Password
              <input
                type="password"
                autoComplete="current-password"
                value={loginPassword}
                onInput={(event) => setLoginPassword(event.currentTarget.value)}
                disabled={loggingIn}
              />
            </label>
          </div>
          <div className="action-row">
            <button type="submit" disabled={loggingIn}>
              {loggingIn ? 'Signing in…' : 'Sign in'}
            </button>
          </div>
        </form>

        {loginError && <div className="error">{loginError}</div>}
        {infoError && <div className="error">{infoError}</div>}
      </main>
    );
  }

  if (isDetailRoute) {
    const labels = taskDetail?.labels || [];
    const deps = taskDetail?.deps || [];
    const backHash = buildHash('/', route.params);

    return (
      <main className="container">
        <header className="header">
          <h1>Task detail</h1>
          <div className="meta">{loadingInfo ? 'Loading server info…' : `project: ${project || '—'}`}</div>
          {authMe?.authenticated && (
            <button type="button" onClick={onLogoutClick}>
              Sign out
            </button>
          )}
        </header>

        <a className="back-link" href={backHash}>
          ← Back to task list
        </a>

        {loadingDetail && <div className="meta">Loading task…</div>}

        {!loadingDetail && taskDetail && (
          <section className="detail-card">
            <h2>{taskDetail.title}</h2>
            <div className="detail-id mono">{taskDetail.id}</div>
            {detailSavingField && <div className="meta">Saving {detailSavingField}…</div>}

            <div className="detail-grid">
              <div>
                <strong>Status:</strong>{' '}
                <select
                  value={taskDetail.status}
                  onChange={onStatusChange}
                  disabled={detailSavingField !== ''}
                >
                  {STATUS_OPTIONS.map((status) => (
                    <option key={status} value={status}>
                      {status}
                    </option>
                  ))}
                </select>
              </div>
              <div>
                <strong>Type:</strong>{' '}
                <select value={taskDetail.type} onChange={onTypeChange} disabled={detailSavingField !== ''}>
                  {TASK_TYPE_OPTIONS.map((taskType) => (
                    <option key={taskType} value={taskType}>
                      {taskType}
                    </option>
                  ))}
                </select>
              </div>
              <div>
                <strong>Priority:</strong>{' '}
                <select value={String(taskDetail.priority)} onChange={onPriorityChange} disabled={detailSavingField !== ''}>
                  {PRIORITY_OPTIONS.map((priority) => (
                    <option key={priority} value={String(priority)}>
                      {priority}
                    </option>
                  ))}
                </select>
              </div>
              <div className="assignee-edit">
                <strong>Assignee:</strong>{' '}
                <input
                  type="text"
                  value={assigneeDraft}
                  onInput={(event) => setAssigneeDraft(event.currentTarget.value)}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter') {
                      event.preventDefault();
                      onAssigneeSave();
                    }
                  }}
                  placeholder="unassigned"
                  disabled={detailSavingField !== ''}
                />
                <button type="button" onClick={onAssigneeSave} disabled={detailSavingField !== ''}>
                  Save
                </button>
              </div>
              <div>
                <strong>Parent:</strong>{' '}
                {taskDetail.parent_id ? <a href={`#/tasks/${taskDetail.parent_id}`}>{taskDetail.parent_id}</a> : '—'}
              </div>
              <div>
                <strong>Spec:</strong> {taskDetail.spec_id || '—'}
              </div>
              <div>
                <strong>Created:</strong> <span className="mono small">{formatTime(taskDetail.created_at)}</span>
              </div>
              <div>
                <strong>Updated:</strong> <span className="mono small">{formatTime(taskDetail.updated_at)}</span>
              </div>
              <div>
                <strong>Closed:</strong> <span className="mono small">{formatTime(taskDetail.closed_at)}</span>
              </div>
            </div>

            <div className="detail-section">
              <div className="detail-section-header">
                <h3>Labels</h3>
              </div>
              {labels.length === 0 ? (
                <div className="meta">No labels.</div>
              ) : (
                <div className="label-row">
                  {labels.map((label) => (
                    <span className="label-chip" key={label}>
                      {label}
                      <button
                        type="button"
                        className="label-remove-btn"
                        onClick={() => onRemoveLabel(label)}
                        disabled={detailSavingField !== ''}
                        aria-label={`Remove label ${label}`}
                      >
                        ×
                      </button>
                    </span>
                  ))}
                </div>
              )}

              <div className="label-add-row">
                <input
                  type="text"
                  value={labelDraft}
                  onInput={(event) => setLabelDraft(event.currentTarget.value)}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter') {
                      event.preventDefault();
                      onAddLabels();
                    }
                  }}
                  placeholder="add labels (comma-separated)"
                  disabled={detailSavingField !== ''}
                />
                <button type="button" onClick={onAddLabels} disabled={detailSavingField !== ''}>
                  Add
                </button>
              </div>
            </div>

            {renderEditableTextSection('Description', 'description')}
            {renderEditableTextSection('Notes', 'notes')}
            {renderEditableTextSection('Acceptance criteria', 'acceptance_criteria')}
            {renderEditableTextSection('Design', 'design')}

            <div className="detail-section">
              <h3>Dependencies</h3>
              {deps.length === 0 ? (
                <div className="meta">No direct dependencies.</div>
              ) : (
                <ul className="dep-list">
                  {deps.map((dep) => (
                    <li key={`${dep.parent_id}-${dep.type}`}>
                      <a href={`#/tasks/${dep.parent_id}`}>{dep.parent_id}</a> <span className="meta">({dep.type})</span>
                    </li>
                  ))}
                </ul>
              )}
            </div>

            <div className="detail-section">
              <h3>Actions</h3>
              <div className="action-row">
                <button
                  type="button"
                  onClick={onCloseTask}
                  disabled={detailSavingField !== '' || taskDetail.status === 'closed' || taskDetail.status === 'tombstone'}
                >
                  Close
                </button>
                <button
                  type="button"
                  onClick={onReopenTask}
                  disabled={detailSavingField !== '' || (taskDetail.status !== 'closed' && taskDetail.status !== 'tombstone')}
                >
                  Reopen
                </button>
                <button
                  type="button"
                  onClick={onTombstoneTask}
                  disabled={detailSavingField !== '' || taskDetail.status === 'tombstone'}
                >
                  Mark tombstone
                </button>
              </div>
            </div>
          </section>
        )}

        {detailSaveError && <div className="error">{detailSaveError}</div>}
        {detailError && <div className="error">{detailError}</div>}
        {infoError && <div className="error">{infoError}</div>}
      </main>
    );
  }

  if (!isListRoute) {
    return (
      <main className="container">
        <h1>grns web ui</h1>
        <div className="meta">Route scaffold active: {route.path}</div>
        <a href="#/">Back to task list</a>
      </main>
    );
  }

  return (
    <main className="container">
      <header className="header">
        <h1>Task list</h1>
        <div className="meta">
          {loadingInfo ? 'Loading server info…' : `project: ${project || '—'} · schema: ${info?.schema_version ?? '—'}`}
        </div>
        {authMe?.authenticated && (
          <button type="button" onClick={onLogoutClick}>
            Sign out
          </button>
        )}
      </header>

      <section className="controls">
        <label className="search">
          <span>Search</span>
          <input
            ref={searchInputRef}
            type="search"
            value={listState.search}
            onInput={onSearchInput}
            placeholder="Search title/description/notes"
          />
        </label>

        <div className="status-row">
          {STATUS_OPTIONS.map((status) => {
            const active = listState.statuses.includes(status);
            return (
              <button
                type="button"
                key={status}
                className={`status-chip ${active ? 'active' : ''}`}
                onClick={() => toggleStatus(status)}
              >
                {status}
              </button>
            );
          })}
        </div>
      </section>

      <section className="list-meta">
        <div>
          {loadingTasks ? 'Loading tasks…' : `Showing ${tasks.length} task(s)`}
          {debouncedSearch && <span> · search: “{debouncedSearch}”</span>}
        </div>
        <label>
          Page size
          <select value={String(listState.limit)} onChange={onLimitChange}>
            {PAGE_SIZE_OPTIONS.map((size) => (
              <option key={size} value={String(size)}>
                {size}
              </option>
            ))}
          </select>
        </label>
      </section>

      <section className="bulk-actions">
        <div className="bulk-summary-row">
          <div className="bulk-summary">Selected: {selectedCount}</div>
          <div className="bulk-summary-actions">
            <button type="button" onClick={() => setShowShortcutHelp(true)}>
              Shortcuts (?)
            </button>
            <button type="button" onClick={clearSelection} disabled={selectedCount === 0 || listActionInFlight || loadingTasks}>
              Clear selection
            </button>
          </div>
        </div>

        <div className="bulk-shortcuts">Shortcuts: ? help · / search · Enter open active · x toggle active · Shift+A select visible · j/k move active</div>

        {selectedByStatus.length > 0 && (
          <div className="bulk-status-counts">
            {selectedByStatus.map((entry) => (
              <span className="bulk-status-chip" key={entry.status}>
                {entry.status}: {entry.count}
              </span>
            ))}
          </div>
        )}

        <div className="bulk-controls">
          <button type="button" onClick={onBulkClose} disabled={selectedCount === 0 || listActionInFlight || loadingTasks}>
            Close selected
          </button>
          <button type="button" onClick={onBulkReopen} disabled={selectedCount === 0 || listActionInFlight || loadingTasks}>
            Reopen selected
          </button>
          <input
            type="text"
            value={bulkLabelDraft}
            onInput={(event) => setBulkLabelDraft(event.currentTarget.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                event.preventDefault();
                onBulkAddLabels();
              }
            }}
            placeholder="labels for selected (comma-separated)"
            disabled={selectedCount === 0 || listActionInFlight || loadingTasks}
          />
          <button type="button" onClick={onBulkAddLabels} disabled={selectedCount === 0 || listActionInFlight || loadingTasks}>
            Add labels
          </button>
        </div>
      </section>

      <section>
        <table className="task-table">
          <thead>
            <tr>
              <th>
                <input
                  type="checkbox"
                  checked={allVisibleSelected}
                  onChange={toggleSelectAllVisible}
                  aria-label="Select all visible tasks"
                  disabled={tasks.length === 0 || loadingTasks || listActionInFlight}
                />
              </th>
              <th>ID</th>
              <th>Title</th>
              <th>Status</th>
              <th>Priority</th>
              <th>Type</th>
              <th>Updated</th>
            </tr>
          </thead>
          <tbody>
            {tasks.map((task, index) => {
              const detailHash = buildHash(`/tasks/${task.id}`, route.params);
              const isActiveRow = index === activeTaskIndex;
              return (
                <tr
                  key={task.id}
                  className={isActiveRow ? 'row-active' : ''}
                  onClick={() => setActiveTaskIndex(index)}
                >
                  <td>
                    <input
                      type="checkbox"
                      checked={selectedSet.has(task.id)}
                      onChange={() => {
                        setActiveTaskIndex(index);
                        toggleTaskSelection(task.id);
                      }}
                      onClick={(event) => event.stopPropagation()}
                      aria-label={`Select task ${task.id}`}
                      disabled={loadingTasks || listActionInFlight}
                    />
                  </td>
                  <td className="mono">{task.id}</td>
                  <td>
                    <a
                      href={detailHash}
                      onClick={(event) => {
                        event.stopPropagation();
                        setActiveTaskIndex(index);
                      }}
                    >
                      {task.title}
                    </a>
                  </td>
                  <td>{task.status}</td>
                  <td>{task.priority}</td>
                  <td>{task.type}</td>
                  <td className="mono small">{formatTime(task.updated_at)}</td>
                </tr>
              );
            })}
            {!loadingTasks && tasks.length === 0 && (
              <tr>
                <td colSpan={7} className="empty">
                  No tasks found for current filters.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </section>

      <section className="pagination">
        <button type="button" onClick={goPrevPage} disabled={!canGoPrev || loadingTasks}>
          ← Prev
        </button>
        <span>Page {page}</span>
        <button type="button" onClick={goNextPage} disabled={!canGoNext || loadingTasks}>
          Next →
        </button>
      </section>

      {showShortcutHelp && (
        <div
          className="shortcut-overlay"
          onClick={() => setShowShortcutHelp(false)}
          role="button"
          tabIndex={0}
          onKeyDown={(event) => {
            if (event.key === 'Escape' || event.key === 'Enter' || event.key === ' ') {
              event.preventDefault();
              setShowShortcutHelp(false);
            }
          }}
        >
          <div className="shortcut-modal" onClick={(event) => event.stopPropagation()}>
            <div className="shortcut-modal-header">
              <h3>Keyboard shortcuts</h3>
              <button type="button" onClick={() => setShowShortcutHelp(false)}>
                Close
              </button>
            </div>
            <ul>
              <li>
                <kbd>?</kbd> toggle shortcut help
              </li>
              <li>
                <kbd>/</kbd> focus search
              </li>
              <li>
                <kbd>Enter</kbd> open active row
              </li>
              <li>
                <kbd>j</kbd>/<kbd>k</kbd> or <kbd>↓</kbd>/<kbd>↑</kbd> move active row
              </li>
              <li>
                <kbd>x</kbd> toggle selection for active row
              </li>
              <li>
                <kbd>Shift</kbd>+<kbd>A</kbd> select/unselect all visible
              </li>
              <li>
                <kbd>Esc</kbd> close this dialog
              </li>
            </ul>
          </div>
        </div>
      )}

      {listActionInfo && <div className="meta">{listActionInfo}</div>}
      {listActionError && <div className="error">{listActionError}</div>}
      {listError && <div className="error">{listError}</div>}
      {infoError && <div className="error">{infoError}</div>}
    </main>
  );
}

