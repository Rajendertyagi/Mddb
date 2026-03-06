import { useState, useEffect } from 'react';
import { X, Settings2, Plus, Trash2 } from 'lucide-react';
import mddbClient from '../lib/mddb-client';

const COLLECTION_TYPES = [
  { value: 'default', label: 'Default', icon: '\uD83D\uDCC1' },
  { value: 'website', label: 'Website', icon: '\uD83C\uDF10' },
  { value: 'images', label: 'Images', icon: '\uD83D\uDDBC\uFE0F' },
  { value: 'audio', label: 'Audio', icon: '\uD83C\uDFB5' },
  { value: 'documents', label: 'Documents', icon: '\uD83D\uDCC4' },
];

export default function CollectionConfigModal({ collection, onClose, onSave }) {
  const [type, setType] = useState('default');
  const [description, setDescription] = useState('');
  const [icon, setIcon] = useState('');
  const [color, setColor] = useState('#3b82f6');
  const [customMeta, setCustomMeta] = useState([]);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);

  useEffect(() => {
    loadConfig();
  }, [collection]);

  const loadConfig = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await mddbClient.getCollectionConfig(collection);
      if (data.configured && data.config) {
        const cfg = data.config;
        setType(cfg.type || 'default');
        setDescription(cfg.description || '');
        setIcon(cfg.icon || '');
        setColor(cfg.color || '#3b82f6');
        const metaEntries = cfg.customMeta
          ? Object.entries(cfg.customMeta).map(([k, v]) => ({ key: k, value: v }))
          : [];
        setCustomMeta(metaEntries);
      }
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const handleSave = async () => {
    setSaving(true);
    setError(null);
    try {
      const metaObj = {};
      customMeta.forEach(({ key, value }) => {
        if (key.trim()) {
          metaObj[key.trim()] = value;
        }
      });
      await mddbClient.setCollectionConfig({
        collection,
        type,
        description,
        icon,
        color,
        customMeta: Object.keys(metaObj).length > 0 ? metaObj : undefined,
      });
      onSave();
      onClose();
    } catch (err) {
      setError(err.message);
    } finally {
      setSaving(false);
    }
  };

  const addMetaEntry = () => {
    setCustomMeta([...customMeta, { key: '', value: '' }]);
  };

  const removeMetaEntry = (index) => {
    setCustomMeta(customMeta.filter((_, i) => i !== index));
  };

  const updateMetaEntry = (index, field, value) => {
    const updated = [...customMeta];
    updated[index] = { ...updated[index], [field]: value };
    setCustomMeta(updated);
  };

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-md max-h-[90vh] overflow-y-auto">
        <div className="p-6">
          <div className="flex items-center justify-between mb-4">
            <div className="flex items-center gap-2">
              <Settings2 className="w-6 h-6 text-blue-600" />
              <h2 className="text-xl font-bold text-gray-900">Collection Settings</h2>
            </div>
            <button onClick={onClose} className="text-gray-400 hover:text-gray-600">
              <X className="w-5 h-5" />
            </button>
          </div>

          <p className="text-sm text-gray-500 mb-4">
            Configure <span className="font-medium text-gray-700">{collection}</span>
          </p>

          {loading ? (
            <div className="flex items-center justify-center py-8">
              <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600"></div>
            </div>
          ) : (
            <div className="space-y-4">
              {/* Type */}
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Type</label>
                <select
                  value={type}
                  onChange={(e) => setType(e.target.value)}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
                >
                  {COLLECTION_TYPES.map((t) => (
                    <option key={t.value} value={t.value}>
                      {t.icon} {t.label}
                    </option>
                  ))}
                </select>
              </div>

              {/* Description */}
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Description</label>
                <textarea
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="Brief description of this collection..."
                  rows={2}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm resize-none"
                />
              </div>

              {/* Icon */}
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Icon (emoji)</label>
                <div className="flex items-center gap-2">
                  <input
                    type="text"
                    value={icon}
                    onChange={(e) => setIcon(e.target.value)}
                    placeholder="e.g. \uD83D\uDCDA"
                    className="flex-1 px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
                    maxLength={4}
                  />
                  {icon && (
                    <span className="text-2xl">{icon}</span>
                  )}
                </div>
              </div>

              {/* Color */}
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Color</label>
                <div className="flex items-center gap-3">
                  <input
                    type="color"
                    value={color}
                    onChange={(e) => setColor(e.target.value)}
                    className="w-10 h-10 rounded border border-gray-300 cursor-pointer"
                  />
                  <input
                    type="text"
                    value={color}
                    onChange={(e) => setColor(e.target.value)}
                    placeholder="#3b82f6"
                    className="flex-1 px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm font-mono"
                  />
                  <div
                    className="w-8 h-8 rounded-full border border-gray-200"
                    style={{ backgroundColor: color }}
                  />
                </div>
              </div>

              {/* Custom Metadata */}
              <div>
                <div className="flex items-center justify-between mb-1">
                  <label className="block text-sm font-medium text-gray-700">Custom Metadata</label>
                  <button
                    type="button"
                    onClick={addMetaEntry}
                    className="flex items-center gap-1 text-xs text-blue-600 hover:text-blue-700"
                  >
                    <Plus className="w-3 h-3" />
                    Add
                  </button>
                </div>
                {customMeta.length === 0 ? (
                  <p className="text-xs text-gray-400">No custom metadata defined</p>
                ) : (
                  <div className="space-y-2">
                    {customMeta.map((entry, idx) => (
                      <div key={idx} className="flex items-center gap-2">
                        <input
                          type="text"
                          value={entry.key}
                          onChange={(e) => updateMetaEntry(idx, 'key', e.target.value)}
                          placeholder="Key"
                          className="flex-1 px-2 py-1.5 border border-gray-300 rounded text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                        />
                        <input
                          type="text"
                          value={entry.value}
                          onChange={(e) => updateMetaEntry(idx, 'value', e.target.value)}
                          placeholder="Value"
                          className="flex-1 px-2 py-1.5 border border-gray-300 rounded text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                        />
                        <button
                          type="button"
                          onClick={() => removeMetaEntry(idx)}
                          className="p-1 text-red-400 hover:text-red-600"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </button>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              {/* Error */}
              {error && (
                <div className="p-3 bg-red-50 border border-red-200 rounded-lg">
                  <p className="text-sm text-red-800">{error}</p>
                </div>
              )}

              {/* Actions */}
              <div className="flex gap-3 pt-2">
                <button
                  type="button"
                  onClick={onClose}
                  className="flex-1 px-4 py-2 text-gray-700 bg-gray-100 hover:bg-gray-200 rounded-lg transition-colors"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  onClick={handleSave}
                  disabled={saving}
                  className="flex-1 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors disabled:opacity-50 flex items-center justify-center gap-2"
                >
                  {saving ? (
                    <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white" />
                  ) : (
                    <Settings2 className="w-4 h-4" />
                  )}
                  Save Settings
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
