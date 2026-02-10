const BASE = '';

function authHeaders() {
  const token = localStorage.getItem('grns_api_token');
  if (!token) {
    return {};
  }
  return { Authorization: `Bearer ${token}` };
}

export async function request(method, path, body) {
  const headers = {
    ...authHeaders(),
  };
  const opts = { method, headers, credentials: 'same-origin' };

  if (body !== undefined) {
    headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(body);
  }

  const res = await fetch(`${BASE}${path}`, opts);
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    const message = err.error || res.statusText;
    const wrapped = new Error(message);
    wrapped.status = res.status;
    wrapped.code = err.code;
    wrapped.errorCode = err.error_code;
    throw wrapped;
  }

  if (res.status === 204) {
    return null;
  }
  return res.json();
}

export function getInfo() {
  return request('GET', '/v1/info');
}

export function getAuthMe() {
  return request('GET', '/v1/auth/me');
}

export function login(username, password) {
  return request('POST', '/v1/auth/login', { username, password });
}

export function logout() {
  return request('POST', '/v1/auth/logout');
}

export function listTasks(project, params = {}) {
  const query = new URLSearchParams(params);
  return request('GET', `/v1/projects/${encodeURIComponent(project)}/tasks?${query.toString()}`);
}

export function getTask(project, id) {
  return request('GET', `/v1/projects/${encodeURIComponent(project)}/tasks/${encodeURIComponent(id)}`);
}

export function updateTask(project, id, patch) {
  return request('PATCH', `/v1/projects/${encodeURIComponent(project)}/tasks/${encodeURIComponent(id)}`, patch);
}

export function addTaskLabels(project, id, labels) {
  return request('POST', `/v1/projects/${encodeURIComponent(project)}/tasks/${encodeURIComponent(id)}/labels`, { labels });
}

export function removeTaskLabels(project, id, labels) {
  return request('DELETE', `/v1/projects/${encodeURIComponent(project)}/tasks/${encodeURIComponent(id)}/labels`, { labels });
}

export function closeTasks(project, ids) {
  return request('POST', `/v1/projects/${encodeURIComponent(project)}/tasks/close`, { ids });
}

export function reopenTasks(project, ids) {
  return request('POST', `/v1/projects/${encodeURIComponent(project)}/tasks/reopen`, { ids });
}
