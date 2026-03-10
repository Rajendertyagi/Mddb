import { useState, useRef } from 'react';
import { Search, AlertCircle, Tag, ChevronDown, ChevronUp, Plus, X, Terminal, Ban } from 'lucide-react';
import { useStore } from '../lib/store';
import mddbClient from '../lib/mddb-client';
import CommandModal from './CommandModal';
import MetaFilterBar from './MetaFilterBar';

export default function FTSSearchPanel() {
  const {
    currentCollection,
    ftsQuery, setFtsQuery,
    ftsLimit, setFtsLimit,
    ftsAlgorithm, setFtsAlgorithm,
    ftsFuzzy, setFtsFuzzy,
    ftsStemming, setFtsStemming,
    ftsSynonyms, setFtsSynonyms,
    ftsFieldWeights, setFtsFieldWeight, removeFtsFieldWeight,
    ftsResults, setFtsResults,
    ftsLoading, setFtsLoading,
    ftsError, setFtsError,
    ftsSearchStats, setFtsSearchStats,
    searchFilterMeta,
    setCurrentDocument,
  } = useStore();

  const [weightsOpen, setWeightsOpen] = useState(true);
  const [newFieldName, setNewFieldName] = useState('');
  const [showCommand, setShowCommand] = useState(false);
  const abortRef = useRef(null);

  const handleCancel = () => {
    if (abortRef.current) {
      abortRef.current.abort();
      abortRef.current = null;
    }
  };

  const handleSearch = async () => {
    if (!currentCollection || !ftsQuery.trim()) return;

    handleCancel();
    const controller = new AbortController();
    abortRef.current = controller;

    setFtsLoading(true);
    setFtsError(null);
    try {
      const data = await mddbClient.ftsSearch({
        collection: currentCollection,
        query: ftsQuery.trim(),
        limit: ftsLimit,
        algorithm: ftsAlgorithm,
        fuzzy: ftsFuzzy,
        disableStem: !ftsStemming,
        disableSynonyms: !ftsSynonyms,
        fieldWeights: ftsAlgorithm === 'bm25f' ? ftsFieldWeights : null,
        filterMeta: searchFilterMeta,
        signal: controller.signal,
      });
      setFtsResults(data.results || []);
      setFtsSearchStats(data.searchStats || null);
    } catch (error) {
      if (error.name === 'AbortError') {
        setFtsError(null);
      } else {
        setFtsError(error.message);
        setFtsResults([]);
        setFtsSearchStats(null);
      }
    } finally {
      setFtsLoading(false);
      abortRef.current = null;
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

  const handleAddField = () => {
    const name = newFieldName.trim();
    if (name && !(name in ftsFieldWeights)) {
      setFtsFieldWeight(name, 1.0);
      setNewFieldName('');
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

  // Normalize score for bar width (max score = 1.0 for display)
  const maxScore = ftsResults.length > 0 ? Math.max(...ftsResults.map(r => r.score)) : 1;

  return (
    <div className="h-full flex flex-col">
      {/* Search Controls */}
      <div className="p-4 border-b border-gray-200 space-y-4">
        <div>
          <label className="block text-xs font-semibold text-gray-500 uppercase tracking-wider mb-1">
            Full-Text Query
          </label>
          <input
            type="text"
            value={ftsQuery}
            onChange={(e) => setFtsQuery(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
            placeholder="Search documents by text..."
            className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
          />
        </div>

        <div className="grid grid-cols-3 gap-3">
          <div>
            <label className="block text-xs font-medium text-gray-600 mb-1">Algorithm</label>
            <select
              value={ftsAlgorithm}
              onChange={(e) => setFtsAlgorithm(e.target.value)}
              className="w-full px-2 py-1.5 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
            >
              <option value="tfidf">TF-IDF</option>
              <option value="bm25">BM25</option>
              <option value="bm25f">BM25F (Field-Weighted)</option>
              <option value="pmisparse">PMISparse (PMI Expansion)</option>
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-600 mb-1">Typo Tolerance</label>
            <select
              value={ftsFuzzy}
              onChange={(e) => setFtsFuzzy(parseInt(e.target.value))}
              className="w-full px-2 py-1.5 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
            >
              <option value={0}>Off</option>
              <option value={1}>Low (1 edit)</option>
              <option value={2}>Medium (2 edits)</option>
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-600 mb-1">
              Limit: {ftsLimit}
            </label>
            <input
              type="range"
              min={1}
              max={200}
              value={ftsLimit}
              onChange={(e) => setFtsLimit(parseInt(e.target.value))}
              className="w-full accent-primary-600"
            />
          </div>
        </div>

        {/* BM25F Field Weights */}
        {ftsAlgorithm === 'bm25f' && (
          <div className="border border-gray-200 rounded-lg">
            <button
              onClick={() => setWeightsOpen(!weightsOpen)}
              className="w-full flex items-center justify-between px-3 py-2 text-xs font-semibold text-gray-600 uppercase tracking-wider hover:bg-gray-50"
            >
              <span>Field Weights</span>
              {weightsOpen ? <ChevronUp className="w-3.5 h-3.5" /> : <ChevronDown className="w-3.5 h-3.5" />}
            </button>
            {weightsOpen && (
              <div className="px-3 pb-3 space-y-2">
                {Object.entries(ftsFieldWeights).map(([field, weight]) => (
                  <div key={field} className="flex items-center space-x-2">
                    <span className="text-xs text-gray-600 w-28 truncate" title={field}>{field}</span>
                    <input
                      type="number"
                      min={0}
                      max={10}
                      step={0.5}
                      value={weight}
                      onChange={(e) => setFtsFieldWeight(field, parseFloat(e.target.value) || 0)}
                      className="w-20 px-2 py-1 border border-gray-300 rounded text-xs text-center focus:outline-none focus:ring-1 focus:ring-primary-500"
                    />
                    <button
                      onClick={() => removeFtsFieldWeight(field)}
                      className="p-0.5 text-gray-400 hover:text-red-500"
                      title="Remove field"
                    >
                      <X className="w-3.5 h-3.5" />
                    </button>
                  </div>
                ))}
                <div className="flex items-center space-x-2 pt-1">
                  <input
                    type="text"
                    value={newFieldName}
                    onChange={(e) => setNewFieldName(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && handleAddField()}
                    placeholder="meta.author"
                    className="flex-1 px-2 py-1 border border-gray-300 rounded text-xs focus:outline-none focus:ring-1 focus:ring-primary-500"
                  />
                  <button
                    onClick={handleAddField}
                    disabled={!newFieldName.trim()}
                    className="flex items-center space-x-1 px-2 py-1 text-xs text-primary-600 hover:bg-primary-50 rounded disabled:opacity-40"
                  >
                    <Plus className="w-3 h-3" />
                    <span>Add</span>
                  </button>
                </div>
              </div>
            )}
          </div>
        )}

        <MetaFilterBar collection={currentCollection} />

        <div className="flex items-center justify-between">
          <div className="flex items-center space-x-4">
            <label className="flex items-center space-x-1.5 text-sm text-gray-600">
              <input
                type="checkbox"
                checked={ftsStemming}
                onChange={(e) => setFtsStemming(e.target.checked)}
                className="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              />
              <span>Stemming</span>
            </label>
            <label className="flex items-center space-x-1.5 text-sm text-gray-600">
              <input
                type="checkbox"
                checked={ftsSynonyms}
                onChange={(e) => setFtsSynonyms(e.target.checked)}
                className="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              />
              <span>Synonyms</span>
            </label>
          </div>
          <div className="flex items-center space-x-2">
            <button
              onClick={() => setShowCommand(true)}
              className="flex items-center space-x-2 px-3 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200 transition-colors"
            >
              <Terminal className="w-4 h-4" />
              <span className="text-sm font-medium">Command</span>
            </button>
            {ftsLoading ? (
              <button
                onClick={handleCancel}
                className="flex items-center space-x-2 px-4 py-2 bg-red-600 text-white rounded-lg hover:bg-red-700 transition-colors"
              >
                <Ban className="w-4 h-4" />
                <span className="text-sm font-medium">Cancel</span>
              </button>
            ) : (
              <button
                onClick={handleSearch}
                disabled={!ftsQuery.trim()}
                className="flex items-center space-x-2 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              >
                <Search className="w-4 h-4" />
                <span className="text-sm font-medium">Search</span>
              </button>
            )}
          </div>
        </div>
      </div>

      {/* Results */}
      <div className="flex-1 overflow-y-auto">
        {ftsResults.length > 0 && (
          <div className="px-4 pt-3 pb-1 flex items-center justify-between">
            <span className="text-xs font-medium text-gray-500">
              {ftsResults.length} result{ftsResults.length !== 1 ? 's' : ''} found
            </span>
            {ftsSearchStats && (
              <span className="text-xs text-gray-400">
                {ftsSearchStats.durationMs}ms | {ftsSearchStats.totalTokens} token{ftsSearchStats.totalTokens !== 1 ? 's' : ''}{ftsSearchStats.queryTerms?.length > 0 ? ` | ${ftsSearchStats.queryTerms.join(', ')}` : ''}
              </span>
            )}
          </div>
        )}

        {ftsError && (
          <div className="m-4 p-3 bg-red-50 border border-red-200 rounded-lg flex items-start space-x-2">
            <AlertCircle className="w-4 h-4 text-red-500 mt-0.5 flex-shrink-0" />
            <p className="text-sm text-red-700">{ftsError}</p>
          </div>
        )}

        {ftsResults.length === 0 && !ftsLoading && !ftsError && ftsQuery && (
          <div className="flex items-center justify-center h-32">
            <p className="text-gray-400 text-sm">No results found</p>
          </div>
        )}

        {ftsResults.length > 0 && (
          <div className="divide-y divide-gray-200">
            {ftsResults.map((result, idx) => {
              const doc = result.document;
              const pct = maxScore > 0 ? Math.round((result.score / maxScore) * 100) : 0;
              return (
                <button
                  key={`${doc?.key}-${doc?.lang}-${idx}`}
                  onClick={() => handleResultClick(result)}
                  className="w-full text-left p-4 hover:bg-gray-50 transition-colors"
                >
                  <div className="flex items-center justify-between mb-2">
                    <div className="flex items-center space-x-2">
                      <span className="inline-flex items-center justify-center w-6 h-6 rounded-full bg-primary-100 text-primary-700 text-xs font-bold">
                        {idx + 1}
                      </span>
                      <h4 className="font-medium text-gray-900 truncate">
                        {doc?.key}
                      </h4>
                      <span className="text-xs text-gray-500">{doc?.lang}</span>
                    </div>
                    <span className="text-sm font-semibold text-primary-600">
                      {result.score.toFixed(4)}
                    </span>
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
        type="fts"
        params={{
          collection: currentCollection,
          query: ftsQuery,
          limit: ftsLimit,
          algorithm: ftsAlgorithm,
          fuzzy: ftsFuzzy,
          disableStem: !ftsStemming,
          disableSynonyms: !ftsSynonyms,
          fieldWeights: ftsAlgorithm === 'bm25f' ? ftsFieldWeights : null,
          filterMeta: searchFilterMeta,
        }}
      />
    </div>
  );
}
