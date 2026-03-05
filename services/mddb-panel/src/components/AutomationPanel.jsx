import { useState, useEffect } from 'react';
import {
  Zap,
  Clock,
  Webhook,
  Plus,
  Trash2,
  Edit3,
  Play,
  RefreshCw,
  ToggleLeft,
  ToggleRight,
  ChevronDown,
  ChevronRight,
} from 'lucide-react';
import { useStore } from '../lib/store';
import mddbClient from '../lib/mddb-client';

const TYPE_CONFIG = {
  webhook: { icon: Webhook, label: 'Webhook', badgeClass: 'bg-blue-100 text-blue-800' },
  trigger: { icon: Zap, label: 'Trigger', badgeClass: 'bg-amber-100 text-amber-800' },
  cron: { icon: Clock, label: 'Cron', badgeClass: 'bg-purple-100 text-purple-800' },
};

const EMPTY_RULE = {
  type: 'webhook',
  name: '',
  enabled: true,
  // webhook fields
  url: '',
  method: 'POST',
  headers: '',
  // trigger fields
  collection: '',
  searchType: 'fts',
  query: '',
  threshold: 50,
  webhookId: '',
  // cron fields
  schedule: '',
  triggerId: '',
};

export default function AutomationPanel() {
  const { stats } = useStore();
  const collections = stats?.collections || [];

  // Data state
  const [rules, setRules] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [filterType, setFilterType] = useState('');

  // Add form state
  const [showAddForm, setShowAddForm] = useState(false);
  const [newRule, setNewRule] = useState({ ...EMPTY_RULE });
  const [adding, setAdding] = useState(false);

  // Edit state
  const [editingId, setEditingId] = useState(null);
  const [editRule, setEditRule] = useState({ ...EMPTY_RULE });
  const [saving, setSaving] = useState(false);

  // Delete / toggle state
  const [deleting, setDeleting] = useState(null);
  const [toggling, setToggling] = useState(null);

  // Test state
  const [testing, setTesting] = useState(null);
  const [testResults, setTestResults] = useState(null);

  useEffect(() => {
    loadRules();
  }, [filterType]);

  const loadRules = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await mddbClient.listAutomation(filterType || undefined);
      setRules(Array.isArray(data) ? data : data.rules || []);
    } catch (err) {
      setError(err.message);
      console.error('Failed to load automation rules:', err);
    } finally {
      setLoading(false);
    }
  };

  // ---- Helpers: extract webhooks / triggers from current rules for dropdowns ----
  const webhookRules = rules.filter((r) => r.type === 'webhook');
  const triggerRules = rules.filter((r) => r.type === 'trigger');

  // ---- Counts for filter tabs ----
  const allCount = rules.length;
  const webhookCount = webhookRules.length;
  const triggerCount = triggerRules.length;
  const cronCount = rules.filter((r) => r.type === 'cron').length;

  // ---- Build payload from form state ----
  const buildPayload = (form) => {
    const payload = {
      type: form.type,
      name: form.name,
      enabled: form.enabled,
    };
    if (form.type === 'webhook') {
      payload.url = form.url;
      payload.method = form.method;
      if (form.headers.trim()) {
        try {
          payload.headers = JSON.parse(form.headers);
        } catch {
          payload.headers = {};
        }
      }
    } else if (form.type === 'trigger') {
      payload.collection = form.collection;
      payload.searchType = form.searchType;
      payload.query = form.query;
      payload.threshold = form.threshold;
      if (form.webhookId) payload.webhookId = form.webhookId;
    } else if (form.type === 'cron') {
      payload.schedule = form.schedule;
      if (form.triggerId) payload.triggerId = form.triggerId;
    }
    return payload;
  };

  // ---- Populate edit form from a rule ----
  const ruleToForm = (rule) => ({
    type: rule.type || 'webhook',
    name: rule.name || '',
    enabled: rule.enabled ?? true,
    url: rule.url || '',
    method: rule.method || 'POST',
    headers: rule.headers ? JSON.stringify(rule.headers, null, 2) : '',
    collection: rule.collection || '',
    searchType: rule.searchType || 'fts',
    query: rule.query || '',
    threshold: rule.threshold ?? 50,
    webhookId: rule.webhookId || '',
    schedule: rule.schedule || '',
    triggerId: rule.triggerId || '',
  });

  // ---- CRUD handlers ----
  const handleCreate = async (e) => {
    e.preventDefault();
    if (!newRule.name.trim()) return;
    setAdding(true);
    setError(null);
    try {
      await mddbClient.createAutomation(buildPayload(newRule));
      setNewRule({ ...EMPTY_RULE });
      setShowAddForm(false);
      await loadRules();
    } catch (err) {
      setError(err.message);
    } finally {
      setAdding(false);
    }
  };

  const handleStartEdit = (rule) => {
    setEditingId(rule.id);
    setEditRule(ruleToForm(rule));
  };

  const handleCancelEdit = () => {
    setEditingId(null);
    setEditRule({ ...EMPTY_RULE });
  };

  const handleSaveEdit = async (id) => {
    setSaving(true);
    setError(null);
    try {
      await mddbClient.updateAutomation(id, buildPayload(editRule));
      setEditingId(null);
      setEditRule({ ...EMPTY_RULE });
      await loadRules();
    } catch (err) {
      setError(err.message);
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (id, name) => {
    if (!confirm(`Delete automation rule "${name}"?`)) return;
    setDeleting(id);
    setError(null);
    try {
      await mddbClient.deleteAutomation(id);
      await loadRules();
    } catch (err) {
      setError(err.message);
    } finally {
      setDeleting(null);
    }
  };

  const handleToggle = async (rule) => {
    setToggling(rule.id);
    setError(null);
    try {
      await mddbClient.updateAutomation(rule.id, { ...rule, enabled: !rule.enabled });
      await loadRules();
    } catch (err) {
      setError(err.message);
    } finally {
      setToggling(null);
    }
  };

  const handleTest = async (id) => {
    setTesting(id);
    setTestResults(null);
    setError(null);
    try {
      const result = await mddbClient.testAutomation(id);
      setTestResults({ id, result });
    } catch (err) {
      setError(err.message);
    } finally {
      setTesting(null);
    }
  };

  // ---- Render helpers ----
  const FilterTab = ({ type, label, count }) => (
    <button
      onClick={() => setFilterType(type)}
      className={`px-4 py-2 text-sm font-medium rounded-lg transition-colors ${
        filterType === type
          ? 'bg-blue-100 text-blue-700'
          : 'text-gray-600 hover:bg-gray-100'
      }`}
    >
      {label} ({count})
    </button>
  );

  const TypeBadge = ({ type }) => {
    const cfg = TYPE_CONFIG[type];
    if (!cfg) return null;
    return (
      <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${cfg.badgeClass}`}>
        {cfg.label}
      </span>
    );
  };

  const TypeIcon = ({ type }) => {
    const cfg = TYPE_CONFIG[type];
    if (!cfg) return null;
    const Icon = cfg.icon;
    return <Icon className="w-4 h-4 text-gray-500" />;
  };

  // ---- Dynamic type-specific fields ----
  const renderTypeFields = (form, setForm) => {
    if (form.type === 'webhook') {
      return (
        <>
          <div>
            <label className="block text-xs text-gray-500 mb-1">URL</label>
            <input
              type="url"
              value={form.url}
              onChange={(e) => setForm({ ...form, url: e.target.value })}
              placeholder="https://example.com/hook"
              className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
            />
          </div>
          <div>
            <label className="block text-xs text-gray-500 mb-1">Method</label>
            <select
              value={form.method}
              onChange={(e) => setForm({ ...form, method: e.target.value })}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
            >
              <option value="POST">POST</option>
              <option value="GET">GET</option>
              <option value="PUT">PUT</option>
            </select>
          </div>
          <div>
            <label className="block text-xs text-gray-500 mb-1">Headers (JSON)</label>
            <textarea
              value={form.headers}
              onChange={(e) => setForm({ ...form, headers: e.target.value })}
              placeholder={'{"Content-Type": "application/json"}'}
              rows={3}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm font-mono"
            />
          </div>
        </>
      );
    }

    if (form.type === 'trigger') {
      return (
        <>
          <div>
            <label className="block text-xs text-gray-500 mb-1">Collection</label>
            <select
              value={form.collection}
              onChange={(e) => setForm({ ...form, collection: e.target.value })}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
            >
              <option value="">Select collection</option>
              {collections.map((c) => (
                <option key={c.name} value={c.name}>
                  {c.name}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-xs text-gray-500 mb-1">Search Type</label>
            <select
              value={form.searchType}
              onChange={(e) => setForm({ ...form, searchType: e.target.value })}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
            >
              <option value="fts">Full-Text Search</option>
              <option value="vector">Vector Search</option>
              <option value="hybrid">Hybrid Search</option>
            </select>
          </div>
          <div>
            <label className="block text-xs text-gray-500 mb-1">Query</label>
            <textarea
              value={form.query}
              onChange={(e) => setForm({ ...form, query: e.target.value })}
              placeholder="Enter search query..."
              rows={2}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
            />
          </div>
          <div>
            <label className="block text-xs text-gray-500 mb-1">
              Threshold: {form.threshold}
            </label>
            <input
              type="range"
              min={0}
              max={100}
              value={form.threshold}
              onChange={(e) => setForm({ ...form, threshold: parseInt(e.target.value, 10) })}
              className="w-full"
            />
          </div>
          <div>
            <label className="block text-xs text-gray-500 mb-1">Webhook</label>
            <select
              value={form.webhookId}
              onChange={(e) => setForm({ ...form, webhookId: e.target.value })}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
            >
              <option value="">Select webhook</option>
              {webhookRules.map((w) => (
                <option key={w.id} value={w.id}>
                  {w.name}
                </option>
              ))}
            </select>
          </div>
        </>
      );
    }

    if (form.type === 'cron') {
      return (
        <>
          <div>
            <label className="block text-xs text-gray-500 mb-1">Schedule</label>
            <input
              type="text"
              value={form.schedule}
              onChange={(e) => setForm({ ...form, schedule: e.target.value })}
              placeholder="0 9 * * *"
              className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm font-mono"
            />
          </div>
          <div>
            <label className="block text-xs text-gray-500 mb-1">Trigger</label>
            <select
              value={form.triggerId}
              onChange={(e) => setForm({ ...form, triggerId: e.target.value })}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
            >
              <option value="">Select trigger</option>
              {triggerRules.map((t) => (
                <option key={t.id} value={t.id}>
                  {t.name}
                </option>
              ))}
            </select>
          </div>
        </>
      );
    }

    return null;
  };

  return (
    <div className="h-full flex flex-col bg-gray-50">
      {/* Header */}
      <div className="bg-white border-b px-6 py-4">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-bold text-gray-900 mb-1">Automation</h1>
            <p className="text-gray-600">Manage webhooks, triggers, and cron schedules</p>
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={() => setShowAddForm(!showAddForm)}
              className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 flex items-center gap-2 text-sm"
            >
              {showAddForm ? (
                <ChevronDown className="w-4 h-4" />
              ) : (
                <Plus className="w-4 h-4" />
              )}
              Add Rule
            </button>
            <button
              onClick={loadRules}
              disabled={loading}
              className="px-4 py-2 bg-gray-100 text-gray-700 rounded hover:bg-gray-200 flex items-center gap-2 disabled:opacity-50"
            >
              <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
              Refresh
            </button>
          </div>
        </div>
      </div>

      {/* Main content */}
      <div className="flex-1 overflow-y-auto p-6">
        <div className="max-w-6xl mx-auto space-y-6">
          {/* Type filter tabs */}
          <div className="flex items-center gap-2">
            <FilterTab type="" label="All" count={allCount} />
            <FilterTab type="webhook" label="Webhooks" count={webhookCount} />
            <FilterTab type="trigger" label="Triggers" count={triggerCount} />
            <FilterTab type="cron" label="Crons" count={cronCount} />
          </div>

          {/* Error alert */}
          {error && (
            <div className="bg-red-50 border border-red-200 rounded-lg p-4 text-red-700 text-sm">
              {error}
            </div>
          )}

          {/* Add form (expandable) */}
          {showAddForm && (
            <div className="bg-white rounded-lg shadow p-6">
              <h2 className="text-lg font-semibold text-gray-900 mb-4 flex items-center gap-2">
                <Plus className="w-5 h-5 text-blue-600" />
                Add Automation Rule
              </h2>
              <form onSubmit={handleCreate} className="space-y-4">
                <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                  <div>
                    <label className="block text-xs text-gray-500 mb-1">Type</label>
                    <select
                      value={newRule.type}
                      onChange={(e) =>
                        setNewRule({ ...EMPTY_RULE, type: e.target.value, name: newRule.name, enabled: newRule.enabled })
                      }
                      className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
                    >
                      <option value="webhook">Webhook</option>
                      <option value="trigger">Trigger</option>
                      <option value="cron">Cron</option>
                    </select>
                  </div>
                  <div>
                    <label className="block text-xs text-gray-500 mb-1">Name</label>
                    <input
                      type="text"
                      value={newRule.name}
                      onChange={(e) => setNewRule({ ...newRule, name: e.target.value })}
                      placeholder="My automation rule"
                      className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
                    />
                  </div>
                  <div className="flex items-end">
                    <label className="flex items-center gap-2 cursor-pointer">
                      <input
                        type="checkbox"
                        checked={newRule.enabled}
                        onChange={(e) => setNewRule({ ...newRule, enabled: e.target.checked })}
                        className="w-4 h-4 text-blue-600 rounded border-gray-300 focus:ring-blue-500"
                      />
                      <span className="text-sm text-gray-700">Enabled</span>
                    </label>
                  </div>
                </div>

                {/* Dynamic fields based on type */}
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  {renderTypeFields(newRule, setNewRule)}
                </div>

                <div className="flex items-center gap-2 pt-2">
                  <button
                    type="submit"
                    disabled={adding || !newRule.name.trim()}
                    className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2 text-sm"
                  >
                    {adding ? (
                      <div className="animate-spin rounded-full h-4 w-4 border-2 border-white border-t-transparent"></div>
                    ) : (
                      <Plus className="w-4 h-4" />
                    )}
                    Create
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setShowAddForm(false);
                      setNewRule({ ...EMPTY_RULE });
                    }}
                    className="px-4 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200 text-sm"
                  >
                    Cancel
                  </button>
                </div>
              </form>
            </div>
          )}

          {/* Rules table */}
          <div className="bg-white rounded-lg shadow">
            <div className="px-6 py-4 border-b border-gray-200">
              <h2 className="text-lg font-semibold text-gray-900">
                Rules ({rules.length})
              </h2>
            </div>

            {loading ? (
              <div className="flex items-center justify-center py-12">
                <div className="text-center">
                  <div className="animate-spin rounded-full h-10 w-10 border-b-2 border-blue-600 mx-auto mb-3"></div>
                  <p className="text-gray-500 text-sm">Loading automation rules...</p>
                </div>
              </div>
            ) : rules.length === 0 ? (
              <div className="p-12 text-center text-gray-500">
                <p>No automation rules found.</p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead>
                    <tr className="bg-gray-50 border-b border-gray-200">
                      <th className="text-left px-6 py-3 text-xs font-medium text-gray-500 uppercase tracking-wider w-10">
                        {/* Icon */}
                      </th>
                      <th className="text-left px-6 py-3 text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Name
                      </th>
                      <th className="text-left px-6 py-3 text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Type
                      </th>
                      <th className="text-left px-6 py-3 text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Status
                      </th>
                      <th className="text-right px-6 py-3 text-xs font-medium text-gray-500 uppercase tracking-wider w-48">
                        Actions
                      </th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200">
                    {rules.map((rule) => (
                      <tr key={rule.id} className="hover:bg-gray-50">
                        {editingId === rule.id ? (
                          /* ---- Inline edit row ---- */
                          <td colSpan={5} className="px-6 py-4">
                            <div className="space-y-4">
                              <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                                <div>
                                  <label className="block text-xs text-gray-500 mb-1">Type</label>
                                  <select
                                    value={editRule.type}
                                    onChange={(e) =>
                                      setEditRule({
                                        ...EMPTY_RULE,
                                        type: e.target.value,
                                        name: editRule.name,
                                        enabled: editRule.enabled,
                                      })
                                    }
                                    className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
                                  >
                                    <option value="webhook">Webhook</option>
                                    <option value="trigger">Trigger</option>
                                    <option value="cron">Cron</option>
                                  </select>
                                </div>
                                <div>
                                  <label className="block text-xs text-gray-500 mb-1">Name</label>
                                  <input
                                    type="text"
                                    value={editRule.name}
                                    onChange={(e) => setEditRule({ ...editRule, name: e.target.value })}
                                    className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
                                  />
                                </div>
                                <div className="flex items-end">
                                  <label className="flex items-center gap-2 cursor-pointer">
                                    <input
                                      type="checkbox"
                                      checked={editRule.enabled}
                                      onChange={(e) => setEditRule({ ...editRule, enabled: e.target.checked })}
                                      className="w-4 h-4 text-blue-600 rounded border-gray-300 focus:ring-blue-500"
                                    />
                                    <span className="text-sm text-gray-700">Enabled</span>
                                  </label>
                                </div>
                              </div>
                              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                                {renderTypeFields(editRule, setEditRule)}
                              </div>
                              <div className="flex items-center gap-2">
                                <button
                                  onClick={() => handleSaveEdit(rule.id)}
                                  disabled={saving}
                                  className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 flex items-center gap-2 text-sm"
                                >
                                  {saving ? (
                                    <div className="animate-spin rounded-full h-4 w-4 border-2 border-white border-t-transparent"></div>
                                  ) : (
                                    <Edit3 className="w-4 h-4" />
                                  )}
                                  Save
                                </button>
                                <button
                                  onClick={handleCancelEdit}
                                  className="px-4 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200 text-sm"
                                >
                                  Cancel
                                </button>
                              </div>
                            </div>
                          </td>
                        ) : (
                          /* ---- Normal display row ---- */
                          <>
                            <td className="px-6 py-4">
                              <TypeIcon type={rule.type} />
                            </td>
                            <td className="px-6 py-4 text-sm font-medium text-gray-900">
                              {rule.name}
                            </td>
                            <td className="px-6 py-4">
                              <TypeBadge type={rule.type} />
                            </td>
                            <td className="px-6 py-4">
                              <button
                                onClick={() => handleToggle(rule)}
                                disabled={toggling === rule.id}
                                className="flex items-center gap-1.5 text-sm transition-colors"
                                title={rule.enabled ? 'Click to disable' : 'Click to enable'}
                              >
                                {toggling === rule.id ? (
                                  <div className="animate-spin rounded-full h-4 w-4 border-2 border-gray-400 border-t-transparent"></div>
                                ) : rule.enabled ? (
                                  <ToggleRight className="w-5 h-5 text-green-600" />
                                ) : (
                                  <ToggleLeft className="w-5 h-5 text-gray-400" />
                                )}
                                <span className={rule.enabled ? 'text-green-700' : 'text-gray-500'}>
                                  {rule.enabled ? 'Enabled' : 'Disabled'}
                                </span>
                              </button>
                            </td>
                            <td className="px-6 py-4 text-right whitespace-nowrap">
                              <div className="flex items-center justify-end gap-2">
                                {rule.type === 'trigger' && (
                                  <button
                                    onClick={() => handleTest(rule.id)}
                                    disabled={testing === rule.id}
                                    className="p-1.5 text-green-600 hover:bg-green-50 rounded transition-colors"
                                    title="Test trigger"
                                  >
                                    {testing === rule.id ? (
                                      <div className="animate-spin rounded-full h-4 w-4 border-2 border-green-600 border-t-transparent"></div>
                                    ) : (
                                      <Play className="w-4 h-4" />
                                    )}
                                  </button>
                                )}
                                <button
                                  onClick={() => handleStartEdit(rule)}
                                  className="p-1.5 text-blue-600 hover:bg-blue-50 rounded transition-colors"
                                  title="Edit"
                                >
                                  <Edit3 className="w-4 h-4" />
                                </button>
                                <button
                                  onClick={() => handleDelete(rule.id, rule.name)}
                                  disabled={deleting === rule.id}
                                  className="p-1.5 text-red-500 hover:bg-red-50 rounded transition-colors"
                                  title="Delete"
                                >
                                  {deleting === rule.id ? (
                                    <div className="animate-spin rounded-full h-4 w-4 border-2 border-red-500 border-t-transparent"></div>
                                  ) : (
                                    <Trash2 className="w-4 h-4" />
                                  )}
                                </button>
                              </div>
                            </td>
                          </>
                        )}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>

          {/* Test results */}
          {testResults && (
            <div className="bg-white rounded-lg shadow p-6">
              <div className="flex items-center justify-between mb-4">
                <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
                  <Play className="w-5 h-5 text-green-600" />
                  Test Results
                </h2>
                <button
                  onClick={() => setTestResults(null)}
                  className="text-sm text-gray-500 hover:text-gray-700"
                >
                  Dismiss
                </button>
              </div>
              <pre className="bg-gray-50 border border-gray-200 rounded-lg p-4 text-sm text-gray-800 overflow-x-auto font-mono">
                {JSON.stringify(testResults.result, null, 2)}
              </pre>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
