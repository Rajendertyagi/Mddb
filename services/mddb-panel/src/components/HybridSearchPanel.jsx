import { useState } from 'react';
import { Search, AlertCircle, Tag, Terminal } from 'lucide-react';
import { useStore } from '../lib/store';
import mddbClient from '../lib/mddb-client';
import CommandModal from './CommandModal';
import MetaFilterBar from './MetaFilterBar';

export default function HybridSearchPanel() {
  const {
    currentCollection,
    hybridQuery, setHybridQuery,
    hybridTopK, setHybridTopK,
    hybridAlpha, setHybridAlpha,
    hybridStrategy, setHybridStrategy,
    hybridRrfK, setHybridRrfK,
    hybridFtsAlgorithm, setHybridFtsAlgorithm,
    hybridVectorAlgorithm, setHybridVectorAlgorithm,
    hybridFuzzy, setHybridFuzzy,
    hybridThreshold, setHybridThreshold,
    hybridResults, setHybridResults,
    hybridLoading, setHybridLoading,
    hybridError, setHybridError,
    searchFilterMeta,
    setCurrentDocument,
  } = useStore();

  const [includeContent, setIncludeContent] = useState(false);
  const [showCommand, setShowCommand] = useState(false);

  const handleSearch = async () => {
    if (!currentCollection || !hybridQuery.trim()) return;

    setHybridLoading(true);
    setHybridError(null);
    try {
      const data = await mddbClient.hybridSearch({
        collection: currentCollection,
        query: hybridQuery.trim(),
        topK: hybridTopK,
        algorithm: hybridFtsAlgorithm,
        vectorAlgorithm: hybridVectorAlgorithm,
        alpha: hybridAlpha,
        strategy: hybridStrategy,
        rrfK: hybridRrfK,
        fuzzy: hybridFuzzy,
        threshold: hybridThreshold,
        includeContent,
        filterMeta: searchFilterMeta,
      });
      setHybridResults(data.results || []);
    } catch (error) {
      setHybridError(error.message);
      setHybridResults([]);
    } finally {
      setHybridLoading(false);
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

  const maxScore = hybridResults.length > 0 ? Math.max(...hybridResults.map(r => r.combinedScore)) : 1;

  return (
    <div className="h-full flex flex-col">
      {/* Search Controls */}
      <div className="p-4 border-b border-gray-200 space-y-4">
        <div>
          <label className="block text-xs font-semibold text-gray-500 uppercase tracking-wider mb-1">
            Hybrid Query
          </label>
          <input
            type="text"
            value={hybridQuery}
            onChange={(e) => setHybridQuery(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
            placeholder="Search with keyword + semantic matching..."
            className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
          />
        </div>

        {/* Strategy */}
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="block text-xs font-medium text-gray-600 mb-1">Strategy</label>
            <select
              value={hybridStrategy}
              onChange={(e) => setHybridStrategy(e.target.value)}
              className="w-full px-2 py-1.5 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
            >
              <option value="alpha">Alpha Blending</option>
              <option value="rrf">RRF (Reciprocal Rank Fusion)</option>
            </select>
          </div>

          {hybridStrategy === 'alpha' && (
            <div>
              <label className="block text-xs font-medium text-gray-600 mb-1">
                Alpha: {hybridAlpha.toFixed(2)}
              </label>
              <input
                type="range"
                min={0}
                max={100}
                value={Math.round(hybridAlpha * 100)}
                onChange={(e) => setHybridAlpha(parseInt(e.target.value) / 100)}
                className="w-full accent-primary-600"
              />
              <div className="flex justify-between text-[10px] text-gray-400 mt-0.5">
                <span>More Keyword</span>
                <span>More Semantic</span>
              </div>
            </div>
          )}

          {hybridStrategy === 'rrf' && (
            <div>
              <label className="block text-xs font-medium text-gray-600 mb-1">
                RRF K
              </label>
              <input
                type="number"
                min={1}
                max={1000}
                value={hybridRrfK}
                onChange={(e) => setHybridRrfK(parseInt(e.target.value) || 60)}
                className="w-full px-2 py-1.5 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
              />
            </div>
          )}
        </div>

        {/* Algorithms */}
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="block text-xs font-medium text-gray-600 mb-1">FTS Algorithm</label>
            <select
              value={hybridFtsAlgorithm}
              onChange={(e) => setHybridFtsAlgorithm(e.target.value)}
              className="w-full px-2 py-1.5 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
            >
              <option value="bm25">BM25</option>
              <option value="bm25f">BM25F (Field-Weighted)</option>
              <option value="pmisparse">PMISparse</option>
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-600 mb-1">Vector Algorithm</label>
            <select
              value={hybridVectorAlgorithm}
              onChange={(e) => setHybridVectorAlgorithm(e.target.value)}
              className="w-full px-2 py-1.5 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
            >
              <option value="flat">Flat (Exact)</option>
              <option value="hnsw">HNSW (Approximate)</option>
              <option value="ivf">IVF (Clustered)</option>
              <option value="pq">PQ (Compressed)</option>
              <option value="sq">SQ (Scalar Quantized)</option>
              <option value="bq">BQ (Binary Quantized)</option>
            </select>
          </div>
        </div>

        {/* Top K, Threshold, Fuzzy */}
        <div className="grid grid-cols-3 gap-3">
          <div>
            <label className="block text-xs font-medium text-gray-600 mb-1">
              Top K: {hybridTopK}
            </label>
            <input
              type="range"
              min={1}
              max={50}
              value={hybridTopK}
              onChange={(e) => setHybridTopK(parseInt(e.target.value))}
              className="w-full accent-primary-600"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-600 mb-1">
              Threshold: {Math.round(hybridThreshold * 100)}%
            </label>
            <input
              type="range"
              min={0}
              max={100}
              value={Math.round(hybridThreshold * 100)}
              onChange={(e) => setHybridThreshold(parseInt(e.target.value) / 100)}
              className="w-full accent-primary-600"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-600 mb-1">Typo Tolerance</label>
            <select
              value={hybridFuzzy}
              onChange={(e) => setHybridFuzzy(parseInt(e.target.value))}
              className="w-full px-2 py-1.5 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
            >
              <option value={0}>Off</option>
              <option value={1}>Low (1 edit)</option>
              <option value={2}>Medium (2 edits)</option>
            </select>
          </div>
        </div>

        <MetaFilterBar collection={currentCollection} />

        <div className="flex items-center justify-between">
          <label className="flex items-center space-x-2 text-sm text-gray-600">
            <input
              type="checkbox"
              checked={includeContent}
              onChange={(e) => setIncludeContent(e.target.checked)}
              className="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            />
            <span>Include content</span>
          </label>
          <div className="flex items-center space-x-2">
            <button
              onClick={() => setShowCommand(true)}
              className="flex items-center space-x-2 px-3 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200 transition-colors"
            >
              <Terminal className="w-4 h-4" />
              <span className="text-sm font-medium">Command</span>
            </button>
            <button
              onClick={handleSearch}
              disabled={hybridLoading || !hybridQuery.trim()}
              className="flex items-center space-x-2 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {hybridLoading ? (
                <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white" />
              ) : (
                <Search className="w-4 h-4" />
              )}
              <span className="text-sm font-medium">Search</span>
            </button>
          </div>
        </div>
      </div>

      {/* Results */}
      <div className="flex-1 overflow-y-auto">
        {hybridResults.length > 0 && (
          <div className="px-4 pt-3 pb-1">
            <span className="text-xs font-medium text-gray-500">
              {hybridResults.length} result{hybridResults.length !== 1 ? 's' : ''} found
            </span>
          </div>
        )}

        {hybridError && (
          <div className="m-4 p-3 bg-red-50 border border-red-200 rounded-lg flex items-start space-x-2">
            <AlertCircle className="w-4 h-4 text-red-500 mt-0.5 flex-shrink-0" />
            <p className="text-sm text-red-700">{hybridError}</p>
          </div>
        )}

        {hybridResults.length === 0 && !hybridLoading && !hybridError && hybridQuery && (
          <div className="flex items-center justify-center h-32">
            <p className="text-gray-400 text-sm">No results found</p>
          </div>
        )}

        {hybridResults.length > 0 && (
          <div className="divide-y divide-gray-200">
            {hybridResults.map((result, idx) => {
              const doc = result.document;
              const pct = maxScore > 0 ? Math.round((result.combinedScore / maxScore) * 100) : 0;
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
                      {result.combinedScore.toFixed(4)}
                    </span>
                  </div>

                  {/* Individual scores */}
                  <div className="flex items-center space-x-3 mb-2">
                    {result.ftsScore !== undefined && (
                      <span className="text-[10px] text-gray-400">
                        FTS: {result.ftsScore.toFixed(4)}
                      </span>
                    )}
                    {result.vectorScore !== undefined && (
                      <span className="text-[10px] text-gray-400">
                        Vector: {result.vectorScore.toFixed(4)}
                      </span>
                    )}
                  </div>

                  {/* Score bar */}
                  <div className="w-full bg-gray-200 rounded-full h-1.5 mb-2">
                    <div
                      className="bg-primary-500 h-1.5 rounded-full transition-all"
                      style={{ width: `${pct}%` }}
                    />
                  </div>

                  {/* Matched terms */}
                  {result.matchedTerms && result.matchedTerms.length > 0 && (
                    <div className="flex flex-wrap gap-1 mt-1">
                      {result.matchedTerms.map((term) => (
                        <span
                          key={term}
                          className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-yellow-100 text-yellow-800"
                        >
                          <Tag className="w-3 h-3 mr-1" />
                          {term}
                        </span>
                      ))}
                    </div>
                  )}

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

      {/* Command Modal */}
      <CommandModal
        isOpen={showCommand}
        onClose={() => setShowCommand(false)}
        type="hybrid"
        params={{
          collection: currentCollection,
          query: hybridQuery,
          topK: hybridTopK,
          algorithm: hybridFtsAlgorithm,
          vectorAlgorithm: hybridVectorAlgorithm,
          alpha: hybridAlpha,
          strategy: hybridStrategy,
          rrfK: hybridRrfK,
          fuzzy: hybridFuzzy,
          threshold: hybridThreshold,
          includeContent,
          filterMeta: searchFilterMeta,
        }}
      />
    </div>
  );
}
