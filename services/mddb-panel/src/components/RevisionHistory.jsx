import { useState, useEffect } from 'react';
import { X, RotateCcw, Eye, Loader2, AlertCircle, Check, Clock } from 'lucide-react';
import { format } from 'date-fns';
import { useStore } from '../lib/store';
import mddbClient from '../lib/mddb-client';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

export default function RevisionHistory({ document, onClose }) {
  const { setCurrentDocument } = useStore();
  const [revisions, setRevisions] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [selectedRevision, setSelectedRevision] = useState(null);
  const [restoring, setRestoring] = useState(false);
  const [restoreSuccess, setRestoreSuccess] = useState(false);

  useEffect(() => {
    loadRevisions();
  }, [document]);

  const loadRevisions = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await mddbClient.getRevisions({
        collection: document.collection,
        key: document.key,
        lang: document.lang,
      });
      setRevisions(data.revisions || []);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const handleRestore = async (timestamp) => {
    setRestoring(true);
    setRestoreSuccess(false);
    try {
      const restored = await mddbClient.restoreRevision({
        collection: document.collection,
        key: document.key,
        lang: document.lang,
        timestamp,
      });
      setCurrentDocument({ ...restored, collection: document.collection });
      setRestoreSuccess(true);
      // Reload revisions to show the new state
      await loadRevisions();
      setTimeout(() => {
        setRestoreSuccess(false);
        onClose();
      }, 1500);
    } catch (err) {
      setError(err.message);
    } finally {
      setRestoring(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-white z-50 flex flex-col">
      {/* Header */}
      <div className="border-b border-gray-200 p-4 flex-shrink-0">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-xl font-semibold text-gray-900">
              Revision History
            </h2>
            <p className="text-sm text-gray-500 mt-1">
              {document.collection} / {document.key} ({document.lang})
            </p>
          </div>
          <button
            onClick={onClose}
            className="p-2 text-gray-400 hover:text-gray-600 rounded-lg hover:bg-gray-100"
          >
            <X className="w-5 h-5" />
          </button>
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-hidden flex">
        {/* Revision List */}
        <div className="w-80 border-r border-gray-200 overflow-y-auto flex-shrink-0">
          {loading && (
            <div className="flex items-center justify-center py-12">
              <Loader2 className="w-6 h-6 text-blue-600 animate-spin" />
              <span className="ml-2 text-sm text-gray-500">Loading revisions...</span>
            </div>
          )}

          {error && (
            <div className="m-4 bg-red-50 border border-red-200 rounded-lg p-3 flex items-start space-x-2">
              <AlertCircle className="w-4 h-4 text-red-600 flex-shrink-0 mt-0.5" />
              <p className="text-sm text-red-700">{error}</p>
            </div>
          )}

          {restoreSuccess && (
            <div className="m-4 bg-green-50 border border-green-200 rounded-lg p-3 flex items-center space-x-2">
              <Check className="w-4 h-4 text-green-600" />
              <p className="text-sm font-medium text-green-900">Revision restored!</p>
            </div>
          )}

          {!loading && revisions.length === 0 && !error && (
            <div className="flex items-center justify-center py-12">
              <p className="text-sm text-gray-500">No revisions found</p>
            </div>
          )}

          {revisions.map((rev, idx) => (
            <button
              key={rev.timestamp}
              onClick={() => setSelectedRevision(rev)}
              className={`w-full text-left p-4 border-b border-gray-100 hover:bg-gray-50 transition-colors ${
                selectedRevision?.timestamp === rev.timestamp ? 'bg-blue-50 border-l-2 border-l-blue-600' : ''
              }`}
            >
              <div className="flex items-center justify-between mb-1">
                <span className="text-xs font-medium text-gray-500">
                  {idx === 0 ? 'Latest' : `Rev #${revisions.length - idx}`}
                </span>
                <Clock className="w-3 h-3 text-gray-400" />
              </div>
              <p className="text-sm font-medium text-gray-900">
                {format(new Date(rev.timestamp * 1000), 'PPpp')}
              </p>
              {rev.meta && Object.keys(rev.meta).length > 0 && (
                <div className="mt-1 flex flex-wrap gap-1">
                  {Object.entries(rev.meta).slice(0, 3).map(([k, v]) => (
                    <span key={k} className="text-xs bg-gray-100 text-gray-600 px-1.5 py-0.5 rounded">
                      {k}: {Array.isArray(v) ? v[0] : v}
                    </span>
                  ))}
                </div>
              )}
            </button>
          ))}
        </div>

        {/* Revision Preview */}
        <div className="flex-1 overflow-y-auto">
          {selectedRevision ? (
            <div className="p-6">
              <div className="flex items-center justify-between mb-4">
                <div>
                  <h3 className="text-lg font-semibold text-gray-900">
                    {format(new Date(selectedRevision.timestamp * 1000), 'PPpp')}
                  </h3>
                  <p className="text-sm text-gray-500 mt-1">
                    Content length: {selectedRevision.contentMd?.length || 0} characters
                  </p>
                </div>
                <button
                  onClick={() => handleRestore(selectedRevision.timestamp)}
                  disabled={restoring}
                  className="flex items-center space-x-2 px-4 py-2 bg-amber-600 text-white rounded-lg hover:bg-amber-700 transition-colors disabled:opacity-50"
                >
                  {restoring ? (
                    <>
                      <Loader2 className="w-4 h-4 animate-spin" />
                      <span>Restoring...</span>
                    </>
                  ) : (
                    <>
                      <RotateCcw className="w-4 h-4" />
                      <span>Restore this version</span>
                    </>
                  )}
                </button>
              </div>

              {selectedRevision.meta && Object.keys(selectedRevision.meta).length > 0 && (
                <div className="mb-4 p-3 bg-gray-50 rounded-lg">
                  <h4 className="text-xs font-semibold text-gray-500 uppercase mb-2">Metadata</h4>
                  <div className="space-y-1">
                    {Object.entries(selectedRevision.meta).map(([k, v]) => (
                      <div key={k} className="text-sm">
                        <span className="font-medium text-gray-700">{k}:</span>{' '}
                        <span className="text-gray-600">{Array.isArray(v) ? v.join(', ') : v}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              <div className="border border-gray-200 rounded-lg p-4 bg-white prose prose-sm max-w-none">
                <ReactMarkdown remarkPlugins={[remarkGfm]}>
                  {selectedRevision.contentMd || '(empty)'}
                </ReactMarkdown>
              </div>
            </div>
          ) : (
            <div className="flex items-center justify-center h-full">
              <div className="text-center">
                <Eye className="w-12 h-12 text-gray-300 mx-auto mb-3" />
                <p className="text-gray-500">Select a revision to preview</p>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
