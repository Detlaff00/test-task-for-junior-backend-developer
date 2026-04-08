const form = document.getElementById('task-form');
const kindEl = document.getElementById('kind');
const ruleFields = document.getElementById('rule-fields');
const resultEl = document.getElementById('form-result');
const calendarEl = document.getElementById('calendar');
const reloadBtn = document.getElementById('reload');
const monthTitleEl = document.getElementById('month-title');
const prevMonthBtn = document.getElementById('prev-month');
const nextMonthBtn = document.getElementById('next-month');
const useTimeEl = document.getElementById('use-time');
const timeFieldsEl = document.getElementById('time-fields');
const startTimeEl = document.getElementById('start_time');
const endTimeEl = document.getElementById('end_time');
const startDateWrapEl = document.getElementById('start-date-wrap');
const submitBtn = form.querySelector('button[type="submit"]');

const specificModalEl = document.getElementById('specific-modal');
const specificCalendarEl = document.getElementById('specific-calendar');
const specificMonthTitleEl = document.getElementById('specific-month-title');
const openSpecificModalBtnId = 'open-specific-modal';
const closeSpecificModalBtn = document.getElementById('close-specific-modal');
const specificPrevMonthBtn = document.getElementById('specific-prev-month');
const specificNextMonthBtn = document.getElementById('specific-next-month');
const createFromSpecificModalBtn = document.getElementById('create-from-specific-modal');
const deleteModalEl = document.getElementById('delete-modal');
const closeDeleteModalBtn = document.getElementById('close-delete-modal');
const deleteSingleBtn = document.getElementById('delete-single-btn');
const deleteSeriesBtn = document.getElementById('delete-series-btn');

let selectedSpecificDates = [];
let tasksCache = [];
let viewedMonth = monthStartUTC(new Date());
let specificViewedMonth = monthStartUTC(new Date());
let pendingDeleteTaskID = null;

const weekdayLabels = ['Пн', 'Вт', 'Ср', 'Чт', 'Пт', 'Сб', 'Вс'];
const monthLabels = [
  'Январь',
  'Февраль',
  'Март',
  'Апрель',
  'Май',
  'Июнь',
  'Июль',
  'Август',
  'Сентябрь',
  'Октябрь',
  'Ноябрь',
  'Декабрь',
];

const statusLabel = {
  new: 'Новая',
  in_progress: 'В работе',
  done: 'Выполнена',
};

const kindLabel = {
  one_time: 'Разовая',
  daily: 'Каждый N-й день',
  monthly_day: 'Число месяца',
  specific_dates: 'Конкретные даты',
  monthly_parity: 'Чётные/нечётные дни',
};

function renderRuleInputs(kind) {
  if (kind !== 'specific_dates') {
    selectedSpecificDates = [];
  }

  startDateWrapEl.style.display = kind === 'specific_dates' ? 'none' : 'block';
  submitBtn.disabled = kind === 'specific_dates';
  submitBtn.textContent = kind === 'specific_dates' ? 'Используйте календарь дат ниже' : 'Создать';

  if (kind === 'daily') {
    ruleFields.innerHTML = '<label>Интервал (каждый N-й день)<input type="number" min="1" id="every_n_days" value="1" /></label>';
    return;
  }
  if (kind === 'monthly_day') {
    ruleFields.innerHTML = `
      <div class="grid-two">
        <label>Число месяца
          <input type="number" min="1" max="31" id="day_of_month" value="1" />
        </label>
        <label>Кол-во месяцев (1..12)
          <input type="number" min="1" max="12" id="months_count" value="1" />
        </label>
      </div>
    `;
    return;
  }
  if (kind === 'specific_dates') {
    selectedSpecificDates = [];
    ruleFields.innerHTML = `
      <div class="row" style="align-items:flex-end; margin-bottom:8px;">
        <div class="meta">Выберите сразу несколько дат во всплывающем календаре.</div>
        <button id="${openSpecificModalBtnId}" type="button">Открыть календарь дат</button>
      </div>
    `;
    const openBtn = document.getElementById(openSpecificModalBtnId);
    if (openBtn) {
      openBtn.addEventListener('click', openSpecificModal);
    }
    return;
  }
  if (kind === 'monthly_parity') {
    ruleFields.innerHTML = '<label>Выбор дней<select id="parity"><option value="odd">Нечётные</option><option value="even">Чётные</option></select></label>';
    return;
  }
  ruleFields.innerHTML = '';
}

function buildPayload() {
  const data = new FormData(form);
  const kind = data.get('recurrence_kind');
  const payload = {
    title: data.get('title')?.trim(),
    description: data.get('description')?.trim() || '',
    status: data.get('status'),
    recurrence_kind: kind,
    all_day: !useTimeEl.checked,
  };

  if (kind !== 'specific_dates') {
    const startDate = data.get('start_date');
    if (startDate) payload.start_date = startDate;
  }

  if (useTimeEl.checked) {
    if (!startTimeEl.value || !endTimeEl.value) {
      throw new Error('Укажите время начала и окончания.');
    }
    payload.start_time = startTimeEl.value;
    payload.end_time = endTimeEl.value;
  }

  if (kind === 'daily') {
    payload.recurrence = { every_n_days: Number(document.getElementById('every_n_days').value) };
  } else if (kind === 'monthly_day') {
    const monthsCountEl = document.getElementById('months_count');
    const monthsCount = monthsCountEl ? Number(monthsCountEl.value) : 1;
    payload.recurrence = {
      day_of_month: Number(document.getElementById('day_of_month').value),
      months_count: Number.isFinite(monthsCount) && monthsCount > 0 ? monthsCount : 1,
    };
  } else if (kind === 'specific_dates') {
    if (selectedSpecificDates.length === 0) {
      throw new Error('Выберите хотя бы одну конкретную дату в календаре.');
    }
    payload.recurrence = { dates: [...selectedSpecificDates] };
  } else if (kind === 'monthly_parity') {
    payload.recurrence = { parity: document.getElementById('parity').value };
  } else {
    payload.recurrence = {};
  }

  return payload;
}

async function createTask(payload) {
  const res = await fetch('/api/v1/tasks', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });

  const body = await res.json();
  if (!res.ok) {
    throw new Error(body.error || 'Ошибка');
  }

  resultEl.classList.add('ok');
  resultEl.textContent = `Создано: ID=${body.id}, дата=${body.scheduled_for}`;

  const createdDate = parseDateKey(body.scheduled_for);
  if (createdDate) viewedMonth = monthStartUTC(createdDate);

  await loadTasks();
}

async function loadTasks() {
  calendarEl.innerHTML = '<div class="meta">Загрузка...</div>';
  const res = await fetch('/api/v1/tasks');
  const tasks = await res.json();
  tasksCache = Array.isArray(tasks) ? tasks : [];
  renderCalendar();
}

function renderCalendar() {
  const year = viewedMonth.getUTCFullYear();
  const month = viewedMonth.getUTCMonth();

  monthTitleEl.textContent = `${monthLabels[month]} ${year}`;

  const map = new Map();
  for (const task of tasksCache) {
    const key = task.scheduled_for;
    if (!map.has(key)) map.set(key, []);
    map.get(key).push(task);
  }

  const firstDay = new Date(Date.UTC(year, month, 1));
  const daysInMonth = new Date(Date.UTC(year, month + 1, 0)).getUTCDate();
  const leading = (firstDay.getUTCDay() + 6) % 7;

  const totalCells = Math.ceil((leading + daysInMonth) / 7) * 7;
  const startGrid = new Date(Date.UTC(year, month, 1 - leading));

  const chunks = [];
  for (const d of weekdayLabels) {
    chunks.push(`<div class="weekday">${d}</div>`);
  }

  for (let i = 0; i < totalCells; i += 1) {
    const date = new Date(Date.UTC(startGrid.getUTCFullYear(), startGrid.getUTCMonth(), startGrid.getUTCDate() + i));
    const key = toDateKey(date);
    const isOutside = date.getUTCMonth() !== month;
    const dayTasks = map.get(key) || [];

    const tasksHtml = dayTasks
      .map(
        (task) => `
          <div class="day-task">
            <strong>${escapeHtml(task.title)}</strong>
            <div>${statusLabel[task.status] || task.status}</div>
            <div class="meta">${kindLabel[task.recurrence_kind] || task.recurrence_kind}</div>
            <div class="meta">${formatTaskTime(task)}</div>
            <button type="button" class="delete-task-btn" data-task-id="${task.id}">Удалить</button>
          </div>
        `,
      )
      .join('');

    chunks.push(`
      <div class="day${isOutside ? ' outside' : ''}">
        <div class="day-number">${date.getUTCDate().toString().padStart(2, '0')}</div>
        ${tasksHtml || '<div class="meta">—</div>'}
      </div>
    `);
  }

  calendarEl.innerHTML = chunks.join('');
}

function openSpecificModal() {
  specificViewedMonth = monthStartUTC(new Date());
  specificModalEl.classList.remove('hidden');
  specificModalEl.setAttribute('aria-hidden', 'false');
  renderSpecificModalCalendar();
}

function closeSpecificModal() {
  specificModalEl.classList.add('hidden');
  specificModalEl.setAttribute('aria-hidden', 'true');
}

function openDeleteModal(taskID) {
  pendingDeleteTaskID = taskID;
  deleteModalEl.classList.remove('hidden');
  deleteModalEl.setAttribute('aria-hidden', 'false');
}

function closeDeleteModal() {
  deleteModalEl.classList.add('hidden');
  deleteModalEl.setAttribute('aria-hidden', 'true');
  pendingDeleteTaskID = null;
}

async function deleteTaskWithScope(scope) {
  if (!pendingDeleteTaskID) return;

  const taskID = pendingDeleteTaskID;
  try {
    const res = await fetch(`/api/v1/tasks/${taskID}?scope=${scope}`, { method: 'DELETE' });
    if (!res.ok) {
      let errText = `Ошибка удаления (HTTP ${res.status})`;
      try {
        const body = await res.json();
        if (body?.error) errText = body.error;
      } catch (_) {}
      throw new Error(errText);
    }

    resultEl.className = 'result ok';
    resultEl.textContent = scope === 'series' ? `Удалена серия #${taskID}` : `Задача #${taskID} удалена`;
    closeDeleteModal();
    await loadTasks();
  } catch (err) {
    resultEl.className = 'result error';
    resultEl.textContent = err.message || 'Не удалось удалить задачу';
  }
}

function renderSpecificModalCalendar() {
  const year = specificViewedMonth.getUTCFullYear();
  const month = specificViewedMonth.getUTCMonth();
  specificMonthTitleEl.textContent = `${monthLabels[month]} ${year}`;

  const firstDay = new Date(Date.UTC(year, month, 1));
  const daysInMonth = new Date(Date.UTC(year, month + 1, 0)).getUTCDate();
  const leading = (firstDay.getUTCDay() + 6) % 7;
  const totalCells = Math.ceil((leading + daysInMonth) / 7) * 7;
  const startGrid = new Date(Date.UTC(year, month, 1 - leading));

  const html = [];
  for (const w of weekdayLabels) {
    html.push(`<div class="weekday">${w}</div>`);
  }

  for (let i = 0; i < totalCells; i += 1) {
    const date = new Date(Date.UTC(startGrid.getUTCFullYear(), startGrid.getUTCMonth(), startGrid.getUTCDate() + i));
    const key = toDateKey(date);
    const selected = selectedSpecificDates.includes(key);
    const outside = date.getUTCMonth() !== month;
    html.push(`<button type="button" class="mini-cell${selected ? ' selected' : ''}${outside ? ' outside' : ''}" data-specific-date="${key}">${String(date.getUTCDate()).padStart(2, '0')}</button>`);
  }

  specificCalendarEl.innerHTML = html.join('');
}

function toggleSpecificDate(value) {
  const idx = selectedSpecificDates.indexOf(value);
  if (idx >= 0) {
    selectedSpecificDates.splice(idx, 1);
  } else {
    selectedSpecificDates.push(value);
    selectedSpecificDates.sort();
  }
}

form.addEventListener('submit', async (e) => {
  e.preventDefault();
  resultEl.className = 'result';
  resultEl.textContent = 'Отправка...';

  try {
    const payload = buildPayload();
    await createTask(payload);
  } catch (err) {
    resultEl.classList.add('error');
    resultEl.textContent = err.message || 'Ошибка сети';
  }
});

createFromSpecificModalBtn.addEventListener('click', async () => {
  resultEl.className = 'result';
  resultEl.textContent = 'Отправка...';

  try {
    const payload = buildPayload();
    await createTask(payload);
    selectedSpecificDates = [];
    closeSpecificModal();
  } catch (err) {
    resultEl.classList.add('error');
    resultEl.textContent = err.message || 'Ошибка сети';
  }
});

kindEl.addEventListener('change', () => renderRuleInputs(kindEl.value));
useTimeEl.addEventListener('change', () => {
  timeFieldsEl.style.display = useTimeEl.checked ? 'grid' : 'none';
  if (!useTimeEl.checked) {
    startTimeEl.value = '';
    endTimeEl.value = '';
  }
});

closeSpecificModalBtn.addEventListener('click', closeSpecificModal);
specificModalEl.addEventListener('click', (e) => {
  const target = e.target;
  if (!(target instanceof HTMLElement)) return;

  if (target.dataset.closeModal === 'true') {
    closeSpecificModal();
    return;
  }

  const selectedDate = target.getAttribute('data-specific-date');
  if (!selectedDate) return;
  toggleSpecificDate(selectedDate);
  renderSpecificModalCalendar();
});

specificPrevMonthBtn.addEventListener('click', () => {
  specificViewedMonth = new Date(Date.UTC(specificViewedMonth.getUTCFullYear(), specificViewedMonth.getUTCMonth() - 1, 1));
  renderSpecificModalCalendar();
});

specificNextMonthBtn.addEventListener('click', () => {
  specificViewedMonth = new Date(Date.UTC(specificViewedMonth.getUTCFullYear(), specificViewedMonth.getUTCMonth() + 1, 1));
  renderSpecificModalCalendar();
});

reloadBtn.addEventListener('click', loadTasks);
calendarEl.addEventListener('click', async (e) => {
  const target = e.target;
  if (!(target instanceof HTMLElement)) return;
  if (!target.classList.contains('delete-task-btn')) return;

  const taskID = Number(target.getAttribute('data-task-id'));
  if (!taskID || Number.isNaN(taskID)) return;
  openDeleteModal(taskID);
});

closeDeleteModalBtn.addEventListener('click', closeDeleteModal);
deleteSingleBtn.addEventListener('click', () => deleteTaskWithScope('single'));
deleteSeriesBtn.addEventListener('click', () => deleteTaskWithScope('series'));
deleteModalEl.addEventListener('click', (e) => {
  const target = e.target;
  if (!(target instanceof HTMLElement)) return;
  if (target.dataset.closeDeleteModal === 'true') {
    closeDeleteModal();
  }
});

prevMonthBtn.addEventListener('click', () => {
  viewedMonth = new Date(Date.UTC(viewedMonth.getUTCFullYear(), viewedMonth.getUTCMonth() - 1, 1));
  renderCalendar();
});

nextMonthBtn.addEventListener('click', () => {
  viewedMonth = new Date(Date.UTC(viewedMonth.getUTCFullYear(), viewedMonth.getUTCMonth() + 1, 1));
  renderCalendar();
});

function monthStartUTC(date) {
  return new Date(Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), 1));
}

function toDateKey(date) {
  const y = date.getUTCFullYear();
  const m = String(date.getUTCMonth() + 1).padStart(2, '0');
  const d = String(date.getUTCDate()).padStart(2, '0');
  return `${y}-${m}-${d}`;
}

function parseDateKey(value) {
  if (typeof value !== 'string') return null;
  const [y, m, d] = value.split('-').map(Number);
  if (!y || !m || !d) return null;
  return new Date(Date.UTC(y, m - 1, d));
}

function formatTaskTime(task) {
  if (task.all_day) return 'Весь день';
  if (task.start_time && task.end_time) return `${task.start_time}-${task.end_time}`;
  return 'Время не задано';
}

function escapeHtml(value) {
  return String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

renderRuleInputs(kindEl.value);
loadTasks();
