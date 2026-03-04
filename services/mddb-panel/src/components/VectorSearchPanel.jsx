import { useState } from 'react';
import { Search, RotateCcw, FileText, Tag, AlertCircle } from 'lucide-react';
import { useStore } from '../lib/store';
import mddbClient from '../lib/mddb-client';

export default function VectorSearchPanel() {
  const {
    currentCollection,
    vectorQuery, setVectorQuery,
    vectorTopK, setVectorTopK,
    vectorThreshold, setVectorThreshold,
    vectorAlgorithm, setVectorAlgorithm,
    vectorResults, setVectorResults,
    vectorLoading, setVectorLoading,
    vectorError, setVectorError,
    setCurrentDocument,
  } = useStore();

  const [includeContent, setIncludeContent] = useState(false);
  const [reindexing, setReindexing] = useState(false);
  const [reindexResult, setReindexResult] = useState(null);

  const handleSearch = async (retryCount = 0) => {
    if (!currentCollection || !vectorQuery.trim()) return;

    setVectorLoading(true);
    setVectorError(null);
    setReindexResult(null);
    try {
      const data = await mddbClient.vectorSearch({
        collection: currentCollection,
        query: vectorQuery.trim(),
        topK: vectorTopK,
        threshold: vectorThreshold,
        includeContent,
        algorithm: vectorAlgorithm,
      });
      setVectorResults(data.results || []);
    } catch (error) {
      const isIndexLoading = error.message && error.message.includes('vector index is loading');
      if (isIndexLoading && retryCount < 3) {
        setVectorError(`Vector index is loading... retrying (${retryCount + 1}/3)`);
        setTimeout(() => handleSearch(retryCount + 1), 2000);
        return;
      }
      setVectorError(isIndexLoading
        ? 'Vector index is still loading. Please wait a moment and try again.'
        : error.message);
      setVectorResults([]);
    } finally {
      if (retryCount === 0 || retryCount >= 3) {
        setVectorLoading(false);
      }
    }
  };

  const handleReindex = async () => {
    if (!currentCollection) return;
    setReindexing(true);
    setReindexResult(null);
    try {
      const result = await mddbClient.vectorReindex({
        collection: currentCollection,
        force: false,
      });
      setReindexResult(result);
    } catch (error) {
      setReindexResult({ error: error.message });
    } finally {
      setReindexing(false);
    }
  };

  const handleResultClick = async (result) => {
    const doc = result.document;
    if (!doc) return;

    const initialDocument = {
      ...doc,
      collection: currentCollection,
      contentMd: doc.contentMd || 'Loading content...',
    };
    setCurrentDocument(initialDocument);

    try {
      const fullDocument = await mddbClient.getDocument({
        collection: currentCollection,
        key: doc.key,
        lang: doc.lang,
      });
      setCurrentDocument({ ...fullDocument, collection: currentCollection });
    } catch (error) {
      setCurrentDocument({
        ...initialDocument,
        contentMd: `Error loading content: ${error.message}`,
      });
    }
  };

  if (!currentCollection) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center">
          <Search className="w-12 h-12 text-gray-300 mx-auto mb-3" />
          <p className="text-gray-500">Select a collection to search</p>
        </div>
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col">
      {/* Search Controls */}
      <div className="p-4 border-b border-gray-200 space-y-4">
        <div>
          <label className="block text-xs font-semibold text-gray-500 uppercase tracking-wider mb-1">
            Semantic Query
          </label>
          <textarea
            value={vectorQuery}
            onChange={(e) => setVectorQuery(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && !e.shiftKey && (e.preventDefault(), handleSearch())}
            placeholder="Describe what you're looking for..."
            rows={2}
            className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 resize-none"
          />
        </div>

        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="block text-xs font-medium text-gray-600 mb-1">
              Top K: {vectorTopK}
            </label>
            <input
              type="range"
              min={1}
              max={50}
              value={vectorTopK}
              onChange={(e) => setVectorTopK(parseInt(e.target.value))}
              className="w-full accent-primary-600"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-600 mb-1">
              Threshold: {Math.round(vectorThreshold * 100)}%
            </label>
            <input
              type="range"
              min={0}
              max={100}
              value={Math.round(vectorThreshold * 100)}
              onChange={(e) => setVectorThreshold(parseInt(e.target.value) / 100)}
              className="w-full accent-primary-600"
            />
          </div>
        </div>

        <div className="flex items-center space-x-3">
          <div>
            <label className="block text-xs font-medium text-gray-600 mb-1">Algorithm</label>
            <select
              value={vectorAlgorithm}
              onChange={(e) => setVectorAlgorithm(e.target.value)}
              className="px-2 py-1.5 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
            >
              <option value="flat">Flat (Exact)</option>
              <option value="hnsw">HNSW (Approximate)</option>
              <option value="ivf">IVF (Clustered)</option>
              <option value="pq">PQ (Compressed)</option>
              <option value="sq">SQ (Scalar Quantized)</option>
              <option value="bq">BQ (Binary Quantized)</option>
            </select>
          </div>
          <label className="flex items-center space-x-2 text-sm text-gray-600 mt-4">
            <input
              type="checkbox"
              checked={includeContent}
              onChange={(e) => setIncludeContent(e.target.checked)}
              className="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            />
            <span>Include content</span>
          </label>
        </div>

        <div className="flex items-center justify-end">

          <button
            onClick={handleSearch}
            disabled={vectorLoading || !vectorQuery.trim()}
            className="flex items-center space-x-2 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {vectorLoading ? (
              <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white" />
            ) : (
              <Search className="w-4 h-4" />
            )}
            <span className="text-sm font-medium">Search</span>
          </button>
        </div>
      </div>

      {/* Results */}
      <div className="flex-1 overflow-y-auto">
        {vectorResults.length > 0 && (
          <div className="px-4 pt-3 pb-1">
            <span className="text-xs font-medium text-gray-500">
              {vectorResults.length} result{vectorResults.length !== 1 ? 's' : ''} found
            </span>
          </div>
        )}

        {vectorError && (
          <div className={`m-4 p-3 rounded-lg flex items-start space-x-2 ${
            vectorError.includes('loading')
              ? 'bg-amber-50 border border-amber-200'
              : 'bg-red-50 border border-red-200'
          }`}>
            {vectorError.includes('loading') ? (
              <RotateCcw className="w-4 h-4 text-amber-500 mt-0.5 flex-shrink-0 animate-spin" />
            ) : (
              <AlertCircle className="w-4 h-4 text-red-500 mt-0.5 flex-shrink-0" />
            )}
            <p className={`text-sm ${vectorError.includes('loading') ? 'text-amber-700' : 'text-red-700'}`}>
              {vectorError}
            </p>
          </div>
        )}

        {vectorResults.length === 0 && !vectorLoading && !vectorError && vectorQuery && (
          <div className="flex items-center justify-center h-32">
            <p className="text-gray-400 text-sm">No results found</p>
          </div>
        )}

        {vectorResults.length > 0 && (
          <div className="divide-y divide-gray-200">
            {vectorResults.map((result, idx) => {
              const doc = result.document;
              const pct = Math.round(result.score * 100);
              return (
                <button
                  key={`${doc?.key}-${doc?.lang}-${idx}`}
                  onClick={() => handleResultClick(result)}
                  className="w-full text-left p-4 hover:bg-gray-50 transition-colors"
                >
                  <div className="flex items-center justify-between mb-2">
                    <div className="flex items-center space-x-2">
                      <span className="inline-flex items-center justify-center w-6 h-6 rounded-full bg-primary-100 text-primary-700 text-xs font-bold">
                        {result.rank || idx + 1}
                      </span>
                      <h4 className="font-medium text-gray-900 truncate">
                        {doc?.key}
                      </h4>
                      <span className="text-xs text-gray-500">{doc?.lang}</span>
                    </div>
                    <span className="text-sm font-semibold text-primary-600">
                      {pct}%
                    </span>
                  </div>

                  {/* Score bar */}
                  <div className="w-full bg-gray-200 rounded-full h-1.5 mb-2">
                    <div
                      className="bg-primary-500 h-1.5 rounded-full transition-all"
                      style={{ width: `${pct}%` }}
                    />
                  </div>

                  {/* Meta tags */}
                  {doc?.meta && Object.keys(doc.meta).length > 0 && (
                    <div className="flex flex-wrap gap-1 mt-1">
                      {Object.entries(doc.meta).slice(0, 3).map(([key, values]) => (
                        <span
                          key={key}
                          className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-gray-100 text-gray-700"
                        >
                          {key}: {Array.isArray(values) ? values.join(', ') : values}
                        </span>
                      ))}
                    </div>
                  )}
                </button>
              );
            })}
          </div>
        )}
      </div>

      {/* Reindex Footer */}
      <div className="p-3 border-t border-gray-200">
        {reindexResult && (
          <div className={`mb-2 p-2 rounded text-xs ${
            reindexResult.error
              ? 'bg-red-50 text-red-700'
              : 'bg-green-50 text-green-700'
          }`}>
            {reindexResult.error
              ? reindexResult.error
              : `Embedded: ${reindexResult.embedded || 0}, Skipped: ${reindexResult.skipped || 0}, Failed: ${reindexResult.failed || 0}`
            }
          </div>
        )}
        <button
          onClick={handleReindex}
          disabled={reindexing}
          className="w-full flex items-center justify-center space-x-2 px-3 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200 transition-colors disabled:opacity-50"
        >
          <RotateCcw className={`w-4 h-4 ${reindexing ? 'animate-spin' : ''}`} />
          <span className="text-sm font-medium">
            {reindexing ? 'Reindexing...' : 'Reindex Embeddings'}
          </span>
        </button>
      </div>
    </div>
  );
}
